package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// decision 是钩子对一次操作的裁决。
type decision int

const (
	allow decision = iota
	deny
)

// verdict 是裁决与给 agent / 人看的理由。
type verdict struct {
	Decision decision
	Reason   string
}

var allowVerdict = verdict{Decision: allow}

func denyf(reason string) verdict { return verdict{Decision: deny, Reason: reason} }

// allowMarkerName 是人工放行标记文件名，放在 git 目录里（不入库、agent 编辑也会被拦）。
// 人执行 `touch "$(git rev-parse --git-path agent-guard-allow)"` 后：收尾的治理违规不再拦截。
const allowMarkerName = "agent-guard-allow"

// fileMutators 是会创建 / 删除 / 改写文件的命令。只在这些命令的参数里查放行标记，
// 避免把脚本或文档正文里提到标记名误判为触碰标记。
var fileMutators = map[string]bool{
	"touch": true, "rm": true, "mv": true, "cp": true, "ln": true, "tee": true,
	"echo": true, "printf": true, "cat": true, "install": true, "truncate": true, "chmod": true,
}

var (
	envFileRe = regexp.MustCompile(`^\.env(rc|\..+)?$`)
	envSafeRe = regexp.MustCompile(`\.(example|sample|template)$`)
	keyFileRe = regexp.MustCompile(`(\.pem|\.key|\.p12|\.pfx)$|^id_(rsa|ed25519|ecdsa)`)
	tagRefRe  = regexp.MustCompile(`^(refs/tags/|v[0-9])`)
	// git commit 的短选项组合里含 n 即 --no-verify（如 -n、-nm、-an）
	commitNoVerifyRe = regexp.MustCompile(`^-[a-zA-Z]*n[a-zA-Z]*$`)
)

// isSecretPath 判断路径是否是凭据类文件（.env*、私钥等），示例模板除外。
func isSecretPath(p string) bool {
	base := filepath.Base(p)
	if envSafeRe.MatchString(base) {
		return false
	}
	return envFileRe.MatchString(base) || keyFileRe.MatchString(base)
}

// evalShell 裁决一条 shell 命令（可含 &&、||、;、| 串联与 bash -c 嵌套）。
// 输入 cmd：原始命令文本；cwd：命令执行目录（相对路径据此解析）；root：仓库根。
// 返回：最严格的一条裁决；解析不了的部分按「放行」处理——本守卫防手滑，不防对抗，git 钩子与 CI 仍兜底。
func evalShell(cmd, cwd, root string) verdict {
	segs := splitSegments(cmd)
	// 提交说明常经 heredoc / -F 传入，逐段切词看不到；只要真有 git commit 段，就对整条命令再查一次
	if strings.Contains(cmd, string(overrideGovernance)) && hasGitCommit(segs) {
		return denyf("「治理豁免」只能由人写进提交说明，agent 不得自行豁免治理守卫")
	}
	for _, seg := range segs {
		if v := evalSegment(seg, cwd, root); v.Decision != allow {
			return v
		}
	}
	return allowVerdict
}

// hasGitCommit 判断是否有某一段真正在执行 git commit（跳过前缀与 git 全局选项）。
func hasGitCommit(segs [][]string) bool {
	for _, seg := range segs {
		t := stripPrefixes(seg)
		if len(t) > 0 && filepath.Base(t[0]) == "git" {
			if sub, _, _, _ := splitGitGlobal(t[1:], ""); sub == "commit" {
				return true
			}
		}
	}
	return false
}

func evalSegment(tokens []string, cwd, root string) verdict {
	tokens = stripPrefixes(tokens)
	if len(tokens) == 0 {
		return allowVerdict
	}
	fileCmd := fileMutators[filepath.Base(tokens[0])]
	for _, t := range tokens {
		if fileCmd && strings.Contains(t, allowMarkerName) {
			return denyf("放行标记只能由人创建或删除，agent 不得触碰 " + allowMarkerName)
		}
		if isSecretPath(t) {
			return denyf("不得读写凭据文件（" + t + "），见 AGENTS.md「通用安全边界」")
		}
	}
	switch filepath.Base(tokens[0]) {
	case "sh", "bash", "zsh":
		for i, t := range tokens {
			if (t == "-c" || t == "-lc") && i+1 < len(tokens) {
				return evalShell(tokens[i+1], cwd, root)
			}
		}
	case "git":
		return evalGit(tokens[1:], cwd)
	case "rm":
		return evalRm(tokens[1:], cwd, root)
	}
	return allowVerdict
}

// stripPrefixes 去掉 VAR=x、env、sudo、command、time 等前缀，露出真正的命令名。
func stripPrefixes(tokens []string) []string {
	for len(tokens) > 0 {
		t := tokens[0]
		switch {
		case t == "env" || t == "sudo" || t == "command" || t == "time" || t == "nohup" || t == "exec":
			tokens = tokens[1:]
		case strings.Contains(t, "=") && !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "="):
			tokens = tokens[1:]
		default:
			return tokens
		}
	}
	return tokens
}

// evalGit 裁决 git 子命令。args 不含开头的 "git"。
func evalGit(args []string, cwd string) verdict {
	sub, rest, _, v := splitGitGlobal(args, cwd)
	if v.Decision != allow || sub == "" {
		return v
	}
	if has(rest, "--no-verify") {
		return denyf("禁止 --no-verify 绕过 git 钩子（AGENTS.md「AI 代理硬性纪律」）")
	}
	switch sub {
	case "commit":
		return evalCommit(rest)
	case "push":
		return evalPush(rest)
	case "tag":
		return evalTag(rest)
	default:
		return evalGitDestructive(sub, rest)
	}
}

// splitGitGlobal 跳过 git 的全局选项（-C dir、-c k=v、--no-pager…），返回子命令、其参数与生效目录。
// -c core.hooksPath=… 直接拒绝。
func splitGitGlobal(args []string, cwd string) (sub string, rest []string, dir string, v verdict) {
	dir = cwd
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if (args[i] == "-C" || args[i] == "-c") && i+1 < len(args) {
			if args[i] == "-C" {
				dir = resolvePath(args[i+1], cwd)
			} else if strings.HasPrefix(strings.ToLower(args[i+1]), "core.hookspath") {
				return "", nil, dir, denyf("不得临时改写 core.hooksPath 绕过 git 钩子")
			}
			i += 2
			continue
		}
		i++
	}
	if i >= len(args) {
		return "", nil, dir, allowVerdict
	}
	return args[i], args[i+1:], dir, allowVerdict
}

// evalGitDestructive 拦会丢弃本地改动或卸载钩子的子命令：reset --hard、clean -f、config core.hooksPath。
func evalGitDestructive(sub string, rest []string) verdict {
	switch sub {
	case "reset":
		if has(rest, "--hard") {
			return denyf("禁止 git reset --hard（会丢弃未提交改动）；需要时请用户亲自执行")
		}
	case "clean":
		for _, a := range rest {
			if a == "--force" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "f")) {
				return denyf("禁止 git clean -f（会删除未跟踪文件）；需要时请用户亲自执行")
			}
		}
	case "config":
		if gitConfigWritesHooksPath(rest) {
			return denyf("不得修改 core.hooksPath（会卸载 git 钩子）；安装请用 make hooks")
		}
	}
	return allowVerdict
}

// gitConfigValueOpts 是 git config 里带独立取值参数的选项（取值不算位置参数）。
var gitConfigValueOpts = map[string]bool{
	"-f": true, "--file": true, "--blob": true, "--type": true, "--default": true, "--comment": true, "--value": true,
}

// gitConfigWriteOpts / gitConfigWriteSubs 是 git config 的写入类选项与子命令（git 2.46+ 子命令形式）。
var (
	gitConfigWriteOpts = map[string]bool{
		"--unset": true, "--unset-all": true, "--add": true, "--replace-all": true,
		"--rename-section": true, "--remove-section": true, "--edit": true, "-e": true,
	}
	gitConfigWriteSubs = map[string]bool{
		"set": true, "unset": true, "rename-section": true, "remove-section": true, "edit": true,
	}
	gitConfigReadOpts = map[string]bool{
		"--get": true, "--get-all": true, "--get-regexp": true, "--get-urlmatch": true,
	}
)

// gitConfigWritesHooksPath 判断一次 git config 调用是否会改写 core.hooksPath。
// 输入 rest：config 之后的参数。
// 返回：写 / 删 core.hooksPath、删改 core 整节 → true；只读（--get 系、get 子命令、单键无值）或不涉及该键 → false。
// 读写分不清时按写处理（宁可多拦）。
// 例：`--get core.hooksPath` → false；`core.hooksPath x` → true；`--remove-section core` → true。
func gitConfigWritesHooksPath(rest []string) bool {
	var positional []string
	write, read := false, false
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case gitConfigValueOpts[a]:
			i++ // 跳过选项取值
		case gitConfigWriteOpts[a]:
			write = true
		case gitConfigReadOpts[a]:
			read = true
		case strings.HasPrefix(a, "-"):
			// --local / --global / --type=bool 等不影响读写判定
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) > 0 {
		switch sub := strings.ToLower(positional[0]); {
		case gitConfigWriteSubs[sub]:
			write, positional = true, positional[1:]
		case sub == "get":
			read, positional = true, positional[1:]
		}
	}
	for _, p := range positional {
		if strings.EqualFold(p, "core") && write {
			return true // 删 / 改 core 整节会连带 hooksPath
		}
	}
	if len(positional) == 0 || !strings.EqualFold(positional[0], "core.hooksPath") {
		return false
	}
	if write {
		return true
	}
	// 只读：显式 --get 系（其后可跟值正则）或单键无值；「键 值」两个位置参数就是写入
	return !read && len(positional) > 1
}

// evalCommit 只拦绕过钩子与自写豁免；在 main 上直接提交是允许的（改代码走 worktree，见 AGENTS.md）。
func evalCommit(args []string) verdict {
	for _, a := range args {
		if commitNoVerifyRe.MatchString(a) {
			return denyf("git commit -n 等同 --no-verify，禁止绕过 git 钩子")
		}
		if strings.Contains(a, string(overrideGovernance)) {
			return denyf("「治理豁免」只能由人写进提交说明，agent 不得自行豁免治理守卫")
		}
	}
	return allowVerdict
}

func evalPush(args []string) verdict {
	for _, a := range args {
		switch {
		case a == "--force" || a == "-f" || strings.HasPrefix(a, "--force-with-lease") || a == "--force-if-includes" || a == "--mirror":
			return denyf("禁止强推（" + a + "）；改写远端历史须用户亲自执行")
		case a == "--delete" || a == "-d" || a == "--prune":
			return denyf("禁止删除远端引用（" + a + "）；须用户亲自执行")
		case a == "--tags" || a == "--follow-tags":
			return denyf("AI 代理不得推送 tag（发布由人执行，见 docs/RELEASE.md）")
		case strings.HasPrefix(a, "-"):
		case strings.HasPrefix(a, "+"):
			return denyf("禁止强推 refspec（" + a + "）")
		case strings.HasPrefix(a, ":"):
			return denyf("禁止删除远端引用（" + a + "）")
		case tagRefRe.MatchString(refDst(a)):
			return denyf("AI 代理不得推送 tag（" + a + "），发布由人执行，见 docs/RELEASE.md")
		}
	}
	return allowVerdict
}

// refDst 取 refspec 的目标端：src:dst → dst；无冒号时即本身。
func refDst(refspec string) string {
	if i := strings.LastIndex(refspec, ":"); i >= 0 {
		return refspec[i+1:]
	}
	return refspec
}

// evalTag 放行列出与创建 tag；删除（-d / --delete）与移动（-f / --force）一律拒绝。
// 推送 tag 属于发布，由 evalPush 拦截。
func evalTag(args []string) verdict {
	for _, a := range args {
		if a == "--delete" || a == "--force" ||
			(strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "df")) {
			return denyf("AI 代理不得删除 / 移动 tag（已推送的 tag 不可改，见 docs/RELEASE.md）")
		}
	}
	return allowVerdict
}

// evalRm 拒绝递归删除仓库外、仓库根本身、.git 内的路径，以及在仓库根用通配符整片删除。
func evalRm(args []string, cwd, root string) verdict {
	recursive := false
	var targets []string
	for _, a := range args {
		switch {
		case a == "--recursive" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "rR")):
			recursive = true
		case strings.HasPrefix(a, "-"):
		default:
			targets = append(targets, a)
		}
	}
	if !recursive {
		return allowVerdict
	}
	for _, t := range targets {
		p := resolvePath(t, cwd)
		dir := p
		if strings.ContainsAny(filepath.Base(p), "*?[") {
			dir = filepath.Dir(p)
			if dir == root {
				return denyf("禁止在仓库根用通配符递归删除（" + t + "）")
			}
		}
		switch {
		case dir == root:
			return denyf("禁止递归删除仓库根（" + t + "）")
		case within(dir, filepath.Join(root, ".git")):
			return denyf("禁止删除 .git 内容（" + t + "）")
		case within(dir, root) || isTempPath(dir):
		default:
			return denyf("禁止递归删除仓库外路径（" + t + "）")
		}
	}
	return allowVerdict
}

// isTempPath 判断 p 是否在系统临时目录下（agent 的草稿区），这类路径允许递归删除。
func isTempPath(p string) bool {
	for _, t := range []string{os.TempDir(), "/tmp", "/private/tmp", "/var/folders", "/private/var/folders"} {
		if t != "" && p != filepath.Clean(t) && within(p, filepath.Clean(t)) {
			return true
		}
	}
	return false
}

// resolvePath 把 p 解析为绝对路径：~ 展开为 HOME，相对路径以 cwd 为基准。
func resolvePath(p, cwd string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

// within 判断 p 是否等于 root 或在 root 之下。
func within(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// splitSegments 把命令文本按 && || ; | 换行 括号 切成若干段，每段再按 shell 引号规则切词。
// 只处理单双引号与反斜杠转义；不展开变量与命令替换。
func splitSegments(cmd string) [][]string {
	var sp segSplitter
	rs := []rune(stripHeredocs(cmd))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case sp.quote != 0:
			i = sp.inQuote(rs, i)
		case r == '\'' || r == '"':
			sp.quote = r
			sp.inTok = true
		case r == '\\' && i+1 < len(rs):
			i++
			sp.write(rs[i])
		case r == ' ' || r == '\t':
			sp.flushTok()
		case strings.ContainsRune("\n;|&()", r):
			sp.flushSeg()
		default:
			sp.write(r)
		}
	}
	sp.flushSeg()
	return sp.segs
}

var heredocRe = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// stripHeredocs 去掉 heredoc 正文（提交说明、脚本源码是数据，不是要执行的命令），保留引出它的那一行。
// 输入 cmd：原始命令文本。返回：去掉正文后的文本；没有结束定界符时正文一直删到末尾。
func stripHeredocs(cmd string) string {
	lines := strings.Split(cmd, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		m := heredocRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		for i+1 < len(lines) && strings.TrimLeft(lines[i+1], "\t") != m[1] {
			i++
		}
		i++ // 跳过结束定界符行
	}
	return strings.Join(out, "\n")
}

// segSplitter 是 splitSegments 的切词状态。
type segSplitter struct {
	segs  [][]string
	cur   []string
	tok   strings.Builder
	inTok bool
	quote rune
}

// inQuote 处理引号内的第 i 个字符，返回处理后的下标（转义会多吃一个字符）。
func (sp *segSplitter) inQuote(rs []rune, i int) int {
	r := rs[i]
	switch {
	case r == sp.quote:
		sp.quote = 0
	case r == '\\' && sp.quote == '"' && i+1 < len(rs):
		i++
		sp.tok.WriteRune(rs[i])
	default:
		sp.tok.WriteRune(r)
	}
	return i
}

func (sp *segSplitter) write(r rune) {
	sp.tok.WriteRune(r)
	sp.inTok = true
}

func (sp *segSplitter) flushTok() {
	if sp.inTok {
		sp.cur = append(sp.cur, sp.tok.String())
		sp.tok.Reset()
		sp.inTok = false
	}
}

func (sp *segSplitter) flushSeg() {
	sp.flushTok()
	if len(sp.cur) > 0 {
		sp.segs = append(sp.segs, sp.cur)
	}
	sp.cur = nil
}
