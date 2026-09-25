package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// overrideKind 是允许人工放行某类违规的提交说明标记。
type overrideKind string

const (
	// overrideGovernance：「治理豁免: <理由>」，放行覆盖率下调、lint 放宽、新增 t.Skip。
	overrideGovernance overrideKind = "治理豁免"
	// overrideBehavior：「行为变更」，放行 characterization 断言的删改。
	overrideBehavior overrideKind = "行为变更"
)

// 规则标识，出现在输出里，方便 grep 与在文档里对照。
const (
	ruleCoverage = "coverage"
	ruleGolangci = "golangci"
	ruleChar     = "characterization"
	ruleSkip     = "t.Skip"
)

const (
	makefilePath = "Makefile"
	golangciPath = ".golangci.yml"
	testGlob     = "*_test.go"
	// selfExclude：守卫自身的测试以字符串形式大量出现 t.Skip 等样本，不参与 Skip 检查
	selfExclude = ":(exclude)tools/agentguard/**"
	zeroSHA     = "0000000000000000000000000000000000000000"
)

// violation 是一条治理违规。
type violation struct {
	Rule     string
	Detail   string
	Override overrideKind // 可用哪个标记放行
}

func (v violation) String() string {
	return fmt.Sprintf("[%s] %s（如确属有意为之，提交说明写「%s: <理由>」）", v.Rule, v.Detail, v.Override)
}

var (
	minCoverageRe = regexp.MustCompile(`(?m)^MIN_COVERAGE\s*\?=\s*([0-9]+(?:\.[0-9]+)?)\s*$`)
	skipCallRe    = regexp.MustCompile(`\b[tbf]\.Skip(f|Now)?\(`)
	govOverrideRe = regexp.MustCompile(`治理豁免\s*[:：]\s*\S+`)
	hunkRe        = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+`)
)

// runGovernance 执行治理检查并打印结果。
// 输入 args：--base REV（对比基线，默认 $GOVERNANCE_BASE → merge-base(HEAD, origin/HEAD) →
// merge-base(HEAD, origin/<main_branch>) → merge-base(HEAD, origin/main) → HEAD）；
// --worktree（agent 收尾模式：只认工作区，不读提交说明里的豁免标记）。
// 返回：0 通过；1 有未放行的违规或运行错误。
func runGovernance(args []string) int {
	fs := flag.NewFlagSet("governance", flag.ContinueOnError)
	base := fs.String("base", os.Getenv("GOVERNANCE_BASE"), "对比基线 revision")
	worktree := fs.Bool("worktree", false, "agent 模式：忽略提交说明里的豁免标记")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	root, err := repoRoot("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentguard: 不在 git 仓库内")
		return 1
	}
	rev := resolveBase(root, *base)
	if rev == "" {
		fmt.Println("governance：没有可对比的基线（新分支首推或空仓库），跳过")
		return 0
	}
	overrideText := os.Getenv("GOVERNANCE_OVERRIDE_TEXT")
	if !*worktree {
		if msgs, err := gitRaw(root, "log", "--format=%B", rev+"..HEAD"); err == nil {
			overrideText += "\n" + msgs
		}
	}
	vs := collectViolations(root, rev, *worktree)
	blocking := filterOverridden(vs, overrideText)
	for _, v := range vs {
		mark := "✗"
		if !contains(blocking, v) {
			mark = "· 已按标记放行"
		}
		fmt.Printf("%s %s\n", mark, v)
	}
	if len(blocking) > 0 {
		fmt.Printf("governance 未通过（基线 %s）。规范见 docs/ENGINEERING_GOVERNANCE.md §4。\n", short(rev))
		return 1
	}
	fmt.Printf("governance 通过 ✓（基线 %s）\n", short(rev))
	return 0
}

// resolveBase 决定对比基线。
// 输入 flagBase：显式基线，可空。
// 返回：可用的 revision；全零 SHA（GitHub push 新分支）或解析失败返回空串，调用方应跳过检查。
func resolveBase(root, flagBase string) string {
	if flagBase == zeroSHA {
		return ""
	}
	if flagBase != "" {
		if _, err := gitOut(root, "rev-parse", "--verify", "-q", flagBase+"^{commit}"); err != nil {
			return ""
		}
		return flagBase
	}
	for _, ref := range mainRefs(root) {
		if mb, err := gitOut(root, "merge-base", "HEAD", ref); err == nil && mb != "" {
			return mb
		}
	}
	if _, err := gitOut(root, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		return "HEAD"
	}
	return ""
}

// mainRefs 按优先级返回主干候选引用，供 resolveBase 取 merge-base。
// 输入 root：仓库根。
// 返回：origin/HEAD（远端默认分支，仓库文件改不了）→ origin/<HEAD 提交里 .agentguard.yml 的 main_branch> → origin/main，已去重。
// 读 HEAD 提交而不是工作区的配置：未提交的改动不能把基线指向当前分支来绕过检查。
func mainRefs(root string) []string {
	src, _ := showAt(root, "HEAD", configPath)
	refs := []string{"origin/HEAD"}
	for _, r := range []string{"origin/" + parseMainBranch(src), "origin/" + defaultMainBranch} {
		if r != refs[len(refs)-1] {
			refs = append(refs, r)
		}
	}
	return refs
}

// collectViolations 对比基线与工作区，收集全部治理违规（不考虑放行标记）。
// 输入 worktree：true 时额外把未跟踪的测试文件当作新增内容检查。
func collectViolations(root, base string, worktree bool) []violation {
	var vs []violation
	if old, ok := showAt(root, base, makefilePath); ok {
		cur, _ := os.ReadFile(filepath.Join(root, makefilePath))
		vs = append(vs, compareCoverage(old, string(cur))...)
	}
	if old, ok := showAt(root, base, golangciPath); ok {
		cur, err := os.ReadFile(filepath.Join(root, golangciPath))
		if err != nil {
			vs = append(vs, violation{ruleGolangci, golangciPath + " 被删除", overrideGovernance})
		} else {
			vs = append(vs, compareGolangci(old, string(cur))...)
		}
	}
	vs = append(vs, charViolations(root, base)...)
	if d, err := gitRaw(root, "diff", "--no-color", "--no-ext-diff", "-U0", base, "--", testGlob, selfExclude); err == nil {
		vs = append(vs, skipViolations(addedLines(d))...)
	}
	if worktree {
		vs = append(vs, untrackedSkipViolations(root)...)
	}
	return vs
}

func untrackedSkipViolations(root string) []violation {
	list, err := gitOut(root, "ls-files", "--others", "--exclude-standard", "--", testGlob, selfExclude)
	if err != nil || list == "" {
		return nil
	}
	var lines []string
	for _, f := range strings.Split(list, "\n") {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		lines = append(lines, strings.Split(string(b), "\n")...)
	}
	return skipViolations(lines)
}

// compareCoverage 检查 MIN_COVERAGE 是否被下调或删除。
// 输入 old / cur：基线与当前的 Makefile 全文。
// 返回：违规列表；基线里没有 MIN_COVERAGE 时不检查。
func compareCoverage(old, cur string) []violation {
	o, ok := parseMinCoverage(old)
	if !ok {
		return nil
	}
	c, ok := parseMinCoverage(cur)
	if !ok {
		return []violation{{ruleCoverage, "Makefile 里的 MIN_COVERAGE 被删除或改成非数字", overrideGovernance}}
	}
	if c < o {
		return []violation{{ruleCoverage, fmt.Sprintf("MIN_COVERAGE 从 %g 下调到 %g（只许上调）", o, c), overrideGovernance}}
	}
	return nil
}

func parseMinCoverage(makefile string) (float64, bool) {
	m := minCoverageRe.FindStringSubmatch(makefile)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	return f, err == nil
}

// charViolations 检查 characterization 用例里既有行的删改。
// 输入 base：对比基线；规则取自基线的 .agentguard.yml（见 config.go）。
// 返回：至多一条违规；未配置、基线里没有该测试文件或 diff 失败时不检查；配置非法时报违规（宁可多拦）。
func charViolations(root, base string) []violation {
	rule, err := loadCharRule(root, base)
	if err != nil {
		return []violation{{ruleChar, err.Error(), overrideBehavior}}
	}
	if rule.path == "" {
		return nil
	}
	old, ok := showAt(root, base, rule.path)
	if !ok {
		return nil
	}
	d, err := gitRaw(root, "diff", "--no-color", "--no-ext-diff", "-U0", base, "--", rule.path)
	if err != nil {
		return nil
	}
	ranges := charFuncRanges(old, rule)
	n := 0
	for _, l := range removedLines(d) {
		if inRanges(l.No, ranges) {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []violation{{ruleChar,
		fmt.Sprintf("%s 的 characterization 用例删除或改动了 %d 行既有内容（TESTING §11：不得迁就实现）", rule.path, n), overrideBehavior}}
}

// lineRange 是闭区间 [From, To] 的行号范围（从 1 起）。
type lineRange struct{ From, To int }

// charFuncRanges 找出源码里 characterization 用例函数（rule.funcRe 命中的顶层函数）占的行号范围。
// 输入 src：基线版本的测试文件全文；rule.funcRe 为 nil 表示整文件都算。
// 返回：各函数从 func 关键字到右花括号的范围；未配置函数正则或源码解析不了时按整文件处理（宁可多拦）。
func charFuncRanges(src string, rule charRule) []lineRange {
	if rule.funcRe == nil {
		return []lineRange{{1, math.MaxInt}}
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rule.path, src, parser.SkipObjectResolution)
	if err != nil {
		return []lineRange{{1, math.MaxInt}}
	}
	var out []lineRange
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || !rule.funcRe.MatchString(fd.Name.Name) {
			continue
		}
		out = append(out, lineRange{fset.Position(fd.Pos()).Line, fset.Position(fd.End()).Line})
	}
	return out
}

func inRanges(n int, rs []lineRange) bool {
	for _, r := range rs {
		if n >= r.From && n <= r.To {
			return true
		}
	}
	return false
}

// diffLine 是 diff 里的一行删除内容及其在旧文件中的行号。
type diffLine struct {
	No   int
	Text string
}

// removedLines 从 -U0 unified diff 里取出被删除的非空行及其旧文件行号（不含 --- 文件头）。
func removedLines(diff string) []diffLine {
	var out []diffLine
	next := 0
	for _, l := range strings.Split(diff, "\n") {
		if m := hunkRe.FindStringSubmatch(l); m != nil {
			next, _ = strconv.Atoi(m[1])
			continue
		}
		if !strings.HasPrefix(l, "-") || strings.HasPrefix(l, "---") {
			continue
		}
		if strings.TrimSpace(l[1:]) != "" {
			out = append(out, diffLine{next, l[1:]})
		}
		next++
	}
	return out
}

// addedLines 从 -U0 unified diff 里取出新增行（不含 +++ 文件头）。
func addedLines(diff string) []string {
	var out []string
	for _, l := range strings.Split(diff, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			out = append(out, l[1:])
		}
	}
	return out
}

// skipViolations 对每条新增的 t.Skip / b.Skip / f.Skip 调用报一条违规。
func skipViolations(lines []string) []violation {
	var vs []violation
	for _, l := range lines {
		if skipCallRe.MatchString(l) {
			vs = append(vs, violation{ruleSkip,
				fmt.Sprintf("新增跳过用例：%s（TESTING §10：flaky 与失败不得 Skip 了事）", strings.TrimSpace(l)), overrideGovernance})
		}
	}
	return vs
}

// filterOverridden 去掉被提交说明 / PR 文本里的标记放行的违规。
// 输入 text：可能含「治理豁免: 理由」或「行为变更」的文本。
// 返回：仍需拦截的违规。「治理豁免」必须带非空理由才生效。
func filterOverridden(vs []violation, text string) []violation {
	gov := govOverrideRe.MatchString(text)
	beh := strings.Contains(text, string(overrideBehavior))
	var out []violation
	for _, v := range vs {
		switch v.Override {
		case overrideGovernance:
			if gov {
				continue
			}
		case overrideBehavior:
			if beh {
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

func contains(vs []violation, v violation) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}
