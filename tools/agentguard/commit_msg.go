package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// commit_msg.go —— check-commit-msg：提交说明规范（AGENTS.md「Git 与 Pull Request 规范」）。
// 标题 <type>(<scope>): <摘要>；scope 在白名单内；feat/fix/refactor/perf 正文带「测试：」行；不带会话尾注。

var (
	commitTitleRe    = regexp.MustCompile(`^(feat|fix|refactor|test|docs|perf|build|ci|chore)(\(([a-z0-9,_/.-]+)\))?!?: \S`)
	testLineRe       = regexp.MustCompile(`(?m)^测试[：:][ \t]*\S`)
	sessionTrailerRe = regexp.MustCompile(`(?mi)^(co-authored-by|claude-session|codex-session):`)
)

// fullSHARe 匹配 40 位十六进制提交 SHA（GitHub 事件里 before 的形态）。
var fullSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// testLineTypes 是正文必须带「测试：」行的提交类型。
var testLineTypes = map[string]bool{"feat": true, "fix": true, "refactor": true, "perf": true}

// toolGeneratedTitleRe 只认 git 实际生成的标题格式；这类提交豁免标题、scope、测试行检查，会话尾注仍检查。
var toolGeneratedTitleRe = regexp.MustCompile(`^(Merge (branch|branches|remote-tracking branch|tag|commit|pull request) |Revert "|(fixup|squash|amend)! )`)

// scissorsLine 是 git commit -v 的剪刀线，其后是 diff，不属于提交说明。
const scissorsLine = "# ------------------------ >8 ------------------------"

// checkCommitMessage 校验一条提交说明。
// 输入 msg：完整提交说明，可含 # 注释行与剪刀线；scopes：允许的 scope 集合，nil 表示不校验 scope。
// 返回：违规描述，按「标题 → scope → 测试行 → 尾注」顺序；合规或工具生成的提交返回 nil。
// 例：("fix(httpx): x\n", {"httpx"}) → ["fix 提交正文缺「测试：<命令及结果>」行（未运行写「测试：未运行（原因）」）"]。
func checkCommitMessage(msg string, scopes map[string]bool) []string {
	lines := commitLines(msg)
	if len(lines) == 0 {
		return []string{"提交说明为空"}
	}
	title := lines[0]
	body := strings.Join(lines[1:], "\n")
	if toolGeneratedTitleRe.MatchString(title) {
		return trailerViolations(body)
	}
	m := commitTitleRe.FindStringSubmatch(title)
	if m == nil {
		return []string{"标题不符合 <type>(<scope>): <摘要>（type 仅限 feat fix refactor test docs perf build ci chore）：" + title}
	}
	var out []string
	if scopes != nil && m[3] != "" {
		for s := range strings.SplitSeq(m[3], ",") {
			if !scopes[s] {
				out = append(out, "scope 不在白名单："+s+"（用包 / 模块名，仓库级 scope 见 .agentguard.yml commit.scopes）")
			}
		}
	}
	if testLineTypes[m[1]] && !testLineRe.MatchString(body) {
		out = append(out, m[1]+" 提交正文缺「测试：<命令及结果>」行（未运行写「测试：未运行（原因）」）")
	}
	return append(out, trailerViolations(body)...)
}

// trailerViolations 检查正文里的会话尾注。
// 输入 body：标题之后的正文。
// 返回：含会话尾注时返回一条违规，否则返回 nil。
func trailerViolations(body string) []string {
	if t := sessionTrailerRe.FindStringSubmatch(body); t != nil {
		return []string{"禁止会话尾注：" + t[1]}
	}
	return nil
}

// commitLines 按 git 默认 cleanup 规则整理提交说明。
// 输入 msg：原始提交说明。
// 返回：去掉 # 注释行、剪刀线及其后内容、开头空行之后的各行（行尾空白已去）。
func commitLines(msg string) []string {
	var out []string
	for line := range strings.SplitSeq(msg, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == scissorsLine {
			break
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if len(out) == 0 && line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// allowedScopes 汇总提交 scope 白名单。
// 输入 root：仓库根；cfg：HEAD 里的配置。
// 返回：cfg.Commit.Scopes 为空时返回 nil（不校验 scope）；否则为配置的 scope，
// 加上仓库内每个含 Go 文件的目录（相对路径与目录名，跳过 testdata/）。列文件失败时返回错误。
func allowedScopes(root string, cfg repoConfig) (map[string]bool, error) {
	if len(cfg.Commit.Scopes) == 0 {
		return nil, nil
	}
	scopes := make(map[string]bool)
	for _, s := range cfg.Commit.Scopes {
		scopes[s] = true
	}
	files, err := presentGoFiles(root)
	if err != nil {
		return nil, err
	}
	for rel := range files {
		if dir := path.Dir(rel); dir != "." {
			scopes[dir] = true
			scopes[path.Base(dir)] = true
		}
	}
	return scopes, nil
}

// runCheckCommitMsg 是 check-commit-msg 子命令。
// 输入 args：一个提交说明文件路径（commit-msg 钩子），或 --range A..B（CI 校验整段提交，跳过合并提交）。
// 返回：0 全部合规，或区间被跳过（起点全零 / 不在本地历史，stdout 提示「跳过检查」）；1 用法错误、不在仓库内、读取失败或有违规（逐条打到 stderr）。
func runCheckCommitMsg(args []string) int {
	var spec, file string
	isRange := false
	switch len(args) {
	case 2:
		pair := [2]string(args)
		first, second := pair[0], pair[1]
		if first != "--range" {
			usage()
			return 1
		}
		isRange, spec = true, second
	case 1:
		one := [1]string(args)
		file = one[0]
		if strings.HasPrefix(file, "-") {
			usage()
			return 1
		}
	default:
		usage()
		return 1
	}
	root, err := repoRoot("")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "check-commit-msg：不在 Git 仓库内")
		return 1
	}
	cfg, err := loadStaticConfig(root)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "check-commit-msg：", err)
		return 1
	}
	scopes, err := allowedScopes(root, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "check-commit-msg：列文件失败：", err)
		return 1
	}
	var violations []string
	if isRange {
		var skipped string
		violations, skipped, err = checkCommitRange(root, spec, scopes)
		if err != nil {
			hint := ""
			if errors.Is(err, os.ErrInvalid) {
				hint = "（应为 A..B）"
			}
			_, _ = fmt.Fprintln(os.Stderr, "check-commit-msg："+err.Error()+hint)
			return 1
		}
		if skipped != "" {
			fmt.Println("check-commit-msg：跳过检查：" + skipped)
			return 0
		}
	} else {
		b, readErr := os.ReadFile(file)
		if readErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, "check-commit-msg：读取提交说明失败：", readErr)
			return 1
		}
		violations = checkCommitMessage(string(b), scopes)
	}
	for _, v := range violations {
		_, _ = fmt.Fprintln(os.Stderr, v)
	}
	if len(violations) > 0 {
		return 1
	}
	fmt.Println("check-commit-msg：提交说明符合规范")
	return 0
}

// checkCommitRange 校验 A..B 区间内每个非合并提交的说明。
// 输入 spec：形如 A..B；A 为全零 SHA（新分支首推），或为 40 位十六进制 SHA 但不在本地历史（force push / 浅克隆）时跳过检查。
// 返回：violations 为带短 SHA 前缀的违规（按提交从旧到新）；skipped 非空表示跳过及原因；spec 非法（errors.Is os.ErrInvalid）、起点无法解析（os.ErrNotExist）或 git 失败时返回错误。
func checkCommitRange(root, spec string, scopes map[string]bool) (violations []string, skipped string, err error) {
	from, _, ok := strings.Cut(spec, "..")
	if !ok || from == "" {
		return nil, "", &os.PathError{Op: "range", Path: spec, Err: os.ErrInvalid}
	}
	if from == zeroSHA {
		return nil, "起点为全零（新分支首推）", nil
	}
	if _, verr := gitOut(root, "rev-parse", "--verify", "-q", from+"^{commit}"); verr != nil {
		if fullSHARe.MatchString(from) {
			return nil, "起点 " + from + " 不在本地历史（force push 或浅克隆）", nil
		}
		return nil, "", &os.PathError{Op: "git rev-parse", Path: from, Err: os.ErrNotExist}
	}
	list, err := gitOut(root, "rev-list", "--no-merges", "--reverse", spec)
	if err != nil {
		return nil, "", &os.PathError{Op: "git rev-list", Path: spec, Err: err}
	}
	for sha := range strings.FieldsSeq(list) {
		msg, logErr := gitRaw(root, "log", "-1", "--format=%B", sha)
		if logErr != nil {
			return nil, "", &os.PathError{Op: "git log", Path: sha, Err: logErr}
		}
		for _, v := range checkCommitMessage(msg, scopes) {
			violations = append(violations, short(sha)+": "+v)
		}
	}
	return violations, "", nil
}
