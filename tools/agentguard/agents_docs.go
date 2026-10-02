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
// 仓库文件只取已跟踪的（含已暂存），不含未跟踪文件，与 CI 检出的内容一致。
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
	listed, err := gitRaw(root, "ls-files", "--cached", "-z")
	if err != nil {
		return nil, &os.PathError{Op: "git ls-files", Path: root, Err: err}
	}
	files := strings.Split(strings.TrimSuffix(listed, "\x00"), "\x00")
	return append(violations, checkCursorGlobs(rules, files)...), nil
}

// isSharedMarker 判断一行是否以指定标记开头（容忍前导空格与制表符）。
// 输入 line：文档中的一行；marker：标记前缀。
// 返回：去掉前导空格 / 制表符后以 marker 开头时为 true。
func isSharedMarker(line, marker string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), marker)
}

// extractSharedBlock 取 AGENTS.md 里两条标记行之间的内容（不含标记行）。
// 输入 doc：AGENTS.md 全文（调用方负责统一换行符）。
// 返回：内容、起始标记行的 0 起始下标、是否找到成对标记；起止标记缺失或顺序颠倒时 ok=false。
func extractSharedBlock(doc string) (block string, startIdx int, ok bool) {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, l := range lines {
		if start < 0 && isSharedMarker(l, sharedStartMarker) {
			start = i
			continue
		}
		if start >= 0 && isSharedMarker(l, sharedEndMarker) {
			return strings.Join(lines[start+1:i], "\n"), start, true
		}
	}
	return "", 0, false
}

// checkSharedRules 比对 AGENTS.md 共享段与内嵌正本（CRLF 视同 LF，忽略首尾空白）。
// 输入 doc：AGENTS.md 全文。
// 返回：至多一条违规；标记数量不对、顺序颠倒或漂移（带文件行号与段内行号）时各报一条。
func checkSharedRules(doc string) []string {
	doc = strings.ReplaceAll(doc, "\r\n", "\n")
	starts, ends := 0, 0
	for _, l := range strings.Split(doc, "\n") {
		if isSharedMarker(l, sharedStartMarker) {
			starts++
		}
		if isSharedMarker(l, sharedEndMarker) {
			ends++
		}
	}
	if starts != 1 || ends != 1 {
		return []string{agentsPath + ": shared-rules 标记须恰好各一个（start " + strconv.Itoa(starts) + " 个，end " + strconv.Itoa(ends) + " 个）"}
	}
	block, startIdx, ok := extractSharedBlock(doc)
	if !ok {
		return []string{agentsPath + ": 缺少成对的 shared-rules:start / shared-rules:end 标记（顺序颠倒）"}
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
	// 文件行号 = 起始标记行号 + 被 TrimSpace 去掉的前导空行数 + 段内行号。
	lead := strings.Count(block[:len(block)-len(strings.TrimLeft(block, " \t\r\n"))], "\n")
	fileLine := startIdx + 1 + lead + line
	return []string{agentsPath + ": shared-rules 段与 agentguard 内嵌正本不一致（" + agentsPath + " 第 " + strconv.Itoa(fileLine) +
		" 行（段内第 " + strconv.Itoa(line) + " 行）起不同）；改共享段须同步 gohttpkit tools/agentguard/shared_rules.md 与两仓 AGENTS.md"}
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

// checkCursorGlobs 检查每个非 alwaysApply 的 .mdc：声明了 globs 的，每个 glob 须被支持且至少匹配一个文件。
// 没有 globs 的规则（仅 description 或手动 @ 引用）合法，不检查；frontmatter 未闭合单独报错。
// 输入 rules：文件名 → 内容；files：仓库已跟踪文件的相对路径。
// 返回：排序后的违规。
func checkCursorGlobs(rules map[string]string, files []string) []string {
	var out []string
	for name, src := range rules {
		rel := cursorRulesDir + "/" + name
		always, globs, ok := parseMDCFrontmatter(src)
		if !ok {
			out = append(out, rel+": frontmatter 未闭合（缺少结尾的 ---）")
			continue
		}
		if always {
			continue
		}
		for _, g := range globs {
			if !globSupported(g) {
				out = append(out, rel+": agentguard 不支持该 glob 语法："+g+"（支持 ** * ? 与字面量）")
				continue
			}
			if !slices.ContainsFunc(files, globRegexp(g).MatchString) {
				out = append(out, rel+": glob "+g+" 匹配不到任何文件（目录改名或规则已失效：改 glob 或删除规则）")
			}
		}
	}
	sort.Strings(out)
	return out
}

// globSupported 判断 glob 是否在 globRegexp 的支持范围内。
// 输入 glob：单个 glob。
// 返回：不含花括号 / 字符类、且不以 / 开头或结尾时为 true。
func globSupported(glob string) bool {
	return !strings.ContainsAny(glob, "{}[]") && !strings.HasPrefix(glob, "/") && !strings.HasSuffix(glob, "/")
}

// unquote 先去掉首尾空白，再去掉两端的单双引号字符（不要求成对）。
// 输入 v：frontmatter 中的一项取值。
// 返回：去引号后的字符串。
func unquote(v string) string {
	return strings.Trim(strings.TrimSpace(v), `"'`)
}

// stripComment 去掉行内 YAML 注释（空格加 # 及其后内容）。
// 输入 v：frontmatter 中的一项取值。
// 返回：注释之前的部分。
func stripComment(v string) string {
	v, _, _ = strings.Cut(v, " #")
	return v
}

// parseMDCFrontmatter 解析 .mdc 开头由 --- 包围的 frontmatter（容忍开头 BOM）。
// 输入 src：.mdc 全文。
// 返回：alwaysApply 是否为 true（忽略大小写、引号与行内 # 注释）；globs 支持单行逗号分隔、YAML 流列表与块列表，
// 含 { 的整行不拆分、原样作为一项交给调用方报不支持；ok=false 表示 frontmatter 缺少结尾 ---。
// 没有 frontmatter 时返回 false, nil, true。
func parseMDCFrontmatter(src string) (always bool, globs []string, ok bool) {
	lines := strings.Split(strings.TrimPrefix(src, "\ufeff"), "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return false, nil, true
	}
	inBlock := false
	for _, l := range lines[1:] {
		l = strings.TrimSpace(l)
		if l == "---" {
			return always, globs, true
		}
		if inBlock && l == "" {
			continue
		}
		if inBlock && strings.HasPrefix(l, "-") {
			if g := unquote(stripComment(strings.TrimPrefix(l, "-"))); g != "" {
				globs = append(globs, g)
			}
			continue
		}
		inBlock = false
		key, val, found := strings.Cut(l, ":")
		if !found {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "alwaysApply":
			always = strings.EqualFold(unquote(stripComment(val)), "true")
		case "globs":
			if val == "" {
				inBlock = true
				continue
			}
			val = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(stripComment(val)), "["), "]")
			if strings.Contains(val, "{") {
				globs = append(globs, unquote(val))
				continue
			}
			for g := range strings.SplitSeq(val, ",") {
				if g = unquote(g); g != "" {
					globs = append(globs, g)
				}
			}
		}
	}
	return false, nil, false
}

// globRegexp 把 Cursor glob 转成锚定正则：** 跨目录，* 与 ? 不跨 /，其余字符按字面匹配。
// 支持范围仅限 ** * ? 与字面量；花括号、字符类、首尾斜杠由 globSupported 在调用前拦下，不在此处理。
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
