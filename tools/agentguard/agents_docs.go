package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// agents_docs.go —— check-agents-docs：跨 agent 规则文件的一致性。
//   shared-rules  AGENTS.md 里 shared-rules:start / end 之间的内容须与 agentguard 内嵌正本逐字一致（insgo 与 gohttpkit 共用这一段）
//   cursor-globs  .cursor/rules/*.mdc 中非 alwaysApply 规则的每个 glob 至少匹配一个仓库文件（防目录改名后规则静默失效）

// sharedRules 是 AGENTS 共享段正本。改共享段 = 改这份文件 + 两仓 AGENTS.md，再发 agentguard 新版本、下游升级。
//
//go:embed shared_rules.md
var sharedRules string

const (
	agentsPath        = "AGENTS.md"
	sharedStartMarker = "<!-- shared-rules:start"
	sharedEndMarker   = "<!-- shared-rules:end -->"
	cursorRulesDir    = ".cursor/rules"
)

// scanAgentsDocs 是 check-agents-docs 的扫描入口。
// 输入 root：Git 仓库根。
// 返回：共享段违规在前、Cursor 规则违规（排序）在后；读 AGENTS.md、读规则目录或列仓库文件失败时返回错误。
func scanAgentsDocs(root string) ([]string, error) {
	doc, err := os.ReadFile(filepath.Join(root, agentsPath))
	if err != nil {
		return nil, &os.PathError{Op: "read", Path: agentsPath, Err: err}
	}
	violations := checkSharedRules(string(doc))
	rules, err := readCursorRules(root)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return violations, nil
	}
	listed, err := gitRaw(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, &os.PathError{Op: "git ls-files", Path: root, Err: err}
	}
	files := strings.Split(strings.TrimSuffix(listed, "\x00"), "\x00")
	return append(violations, checkCursorGlobs(rules, files)...), nil
}

// extractSharedBlock 取 AGENTS.md 里两条标记行之间的内容（不含标记行）。
// 输入 doc：AGENTS.md 全文。
// 返回：内容与是否找到成对标记；起止标记缺失或顺序颠倒时 ok=false。
func extractSharedBlock(doc string) (string, bool) {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, l := range lines {
		if start < 0 && strings.HasPrefix(l, sharedStartMarker) {
			start = i
			continue
		}
		if start >= 0 && strings.HasPrefix(l, sharedEndMarker) {
			return strings.Join(lines[start+1:i], "\n"), true
		}
	}
	return "", false
}

// checkSharedRules 比对 AGENTS.md 共享段与内嵌正本（忽略首尾空白）。
// 输入 doc：AGENTS.md 全文。
// 返回：至多一条违规，漂移时带第一处不同的行号。
func checkSharedRules(doc string) []string {
	block, ok := extractSharedBlock(doc)
	if !ok {
		return []string{agentsPath + ": 缺少成对的 shared-rules:start / shared-rules:end 标记"}
	}
	got, want := strings.TrimSpace(block), strings.TrimSpace(sharedRules)
	if got == want {
		return nil
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	line := min(len(gl), len(wl)) + 1
	for i := range min(len(gl), len(wl)) {
		if gl[i] != wl[i] {
			line = i + 1
			break
		}
	}
	return []string{agentsPath + ": shared-rules 段与 agentguard 内嵌正本不一致（段内第 " + strconv.Itoa(line) +
		" 行起不同）；改共享段须同步 gohttpkit tools/agentguard/shared_rules.md 与两仓 AGENTS.md"}
}

// readCursorRules 读 .cursor/rules 下全部 .mdc。
// 输入 root：Git 仓库根。
// 返回：文件名 → 内容；目录不存在时返回空 map；读失败返回错误。
func readCursorRules(root string) (map[string]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, cursorRulesDir))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, &os.PathError{Op: "readdir", Path: cursorRulesDir, Err: err}
	}
	rules := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mdc") {
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(root, cursorRulesDir, e.Name()))
		if readErr != nil {
			return nil, &os.PathError{Op: "read", Path: cursorRulesDir + "/" + e.Name(), Err: readErr}
		}
		rules[e.Name()] = string(b)
	}
	return rules, nil
}

// checkCursorGlobs 检查每个非 alwaysApply 的 .mdc：globs 非空，且每个 glob 至少匹配一个文件。
// 输入 rules：文件名 → 内容；files：仓库文件（已跟踪与未跟踪、不含忽略）的相对路径。
// 返回：排序后的违规。
func checkCursorGlobs(rules map[string]string, files []string) []string {
	var out []string
	for name, src := range rules {
		rel := cursorRulesDir + "/" + name
		always, globs := parseMDCFrontmatter(src)
		if always {
			continue
		}
		if len(globs) == 0 {
			out = append(out, rel+": 既不是 alwaysApply 也没有 globs，规则永远不会被加载")
			continue
		}
		for _, g := range globs {
			if !slices.ContainsFunc(files, globRegexp(g).MatchString) {
				out = append(out, rel+": glob "+g+" 匹配不到任何文件（目录改名或规则已失效：改 glob 或删除规则）")
			}
		}
	}
	sort.Strings(out)
	return out
}

// parseMDCFrontmatter 解析 .mdc 开头由 --- 包围的 frontmatter。
// 输入 src：.mdc 全文。
// 返回：alwaysApply 是否为 true；globs 按逗号拆分、去空白与引号后的列表。没有 frontmatter 时返回 false, nil。
func parseMDCFrontmatter(src string) (always bool, globs []string) {
	lines := strings.Split(src, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return false, nil
	}
	for _, l := range lines[1:] {
		l = strings.TrimSpace(l)
		if l == "---" {
			break
		}
		key, val, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "alwaysApply":
			always = val == "true"
		case "globs":
			for g := range strings.SplitSeq(val, ",") {
				if g = strings.Trim(strings.TrimSpace(g), `"'`); g != "" {
					globs = append(globs, g)
				}
			}
		}
	}
	return always, globs
}

// globRegexp 把 Cursor glob 转成锚定正则：** 跨目录，* 与 ? 不跨 /，其余字符按字面匹配。
// 输入 glob：Cursor 规则里的 glob 写法。
// 返回：锚定的正则。例：android/** → ^android/.*$；**/*_test.go → ^(?:.*/)?[^/]*_test\.go$。
func globRegexp(glob string) *regexp.Regexp {
	rs := []rune(glob)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '*' && i+1 < len(rs) && rs[i+1] == '*':
			i++
			if i+1 < len(rs) && rs[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case rs[i] == '*':
			b.WriteString("[^/]*")
		case rs[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(rs[i])))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
