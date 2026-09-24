package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const baseGolangci = `version: "2"
run:
  tests: true
linters:
  default: standard
  enable:
    - errorlint
    - nolintlint
  settings:
    gocyclo:
      min-complexity: 20
    goconst:
      min-occurrences: 2
      ignore-calls: false
    nolintlint:
      require-specific: true
      require-explanation: true
      allow-unused: false
    forbidigo:
      forbid:
        - pattern: '^fmt\.Print'
          msg: "用 logger"
    gosec:
      excludes:
        - G404
  exclusions:
    rules:
      - path: _test\.go
        linters: [gocyclo]
`

func TestCompareGolangci_各类放宽都报(t *testing.T) {
	cases := []struct {
		name, old, new, want string
	}{
		{"关闭 linter", "- errorlint\n", "", "关闭了 linter：errorlint"},
		{"改 default", "default: standard", "default: none", "linters.default"},
		{"新增豁免规则", "        linters: [gocyclo]\n", "        linters: [gocyclo]\n      - path: httpx/\n        linters: [errorlint]\n", "新增豁免"},
		{"已有豁免追加 linter", "linters: [gocyclo]", "linters: [gocyclo, gosec]", "新增豁免"},
		{"调高复杂度阈值", "min-complexity: 20", "min-complexity: 30", "gocyclo.min-complexity"},
		{"删掉复杂度阈值", "      min-complexity: 20\n", "", "gocyclo.min-complexity"},
		{"goconst ignore-calls 放开", "ignore-calls: false", "ignore-calls: true", "ignore-calls"},
		{"nolintlint 不要求理由", "require-explanation: true", "require-explanation: false", "require-explanation"},
		{"nolintlint 允许失效豁免", "allow-unused: false", "allow-unused: true", "allow-unused"},
		{"不再检查测试文件", "tests: true", "tests: false", "run.tests"},
		{"删 forbidigo 规则", "        - pattern: '^fmt\\.Print'\n          msg: \"用 logger\"\n", "", "forbidigo"},
		{"gosec 新增排除", "        - G404\n", "        - G404\n        - G101\n", "G101"},
		{"issues 只看新增", "  exclusions:", "issues:\n  new: true\nlinters2:\n  exclusions:", "issues"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cur := strings.Replace(baseGolangci, c.old, c.new, 1)
			if cur == baseGolangci {
				t.Fatalf("用例替换未生效：%q", c.old)
			}
			vs := compareGolangci(baseGolangci, cur)
			t.Logf("%s → %v", c.name, vs)
			if !anyContains(vs, c.want) {
				t.Fatalf("期望含 %q 的违规，得到 %v", c.want, vs)
			}
			for _, v := range vs {
				if v.Rule != ruleGolangci || v.Override != overrideGovernance {
					t.Fatalf("违规标识错误：%+v", v)
				}
			}
		})
	}
}

func TestCompareGolangci_收紧与不变不报(t *testing.T) {
	tighter := strings.NewReplacer(
		"    - nolintlint\n", "    - nolintlint\n    - gosec\n",
		"min-complexity: 20", "min-complexity: 15",
		"        - G404\n", "",
	).Replace(baseGolangci)
	for name, cur := range map[string]string{"不变": baseGolangci, "收紧": tighter} {
		if vs := compareGolangci(baseGolangci, cur); len(vs) != 0 {
			t.Fatalf("%s 不应报违规，得到 %v", name, vs)
		}
	}
}

func TestCompareGolangci_解析失败(t *testing.T) {
	if vs := compareGolangci("linters: [\n", baseGolangci); vs != nil {
		t.Fatalf("基线无法解析应跳过，得到 %v", vs)
	}
	if vs := compareGolangci("just a string", baseGolangci); vs != nil {
		t.Fatalf("基线无法解析应跳过，得到 %v", vs)
	}
	vs := compareGolangci(baseGolangci, "linters: [\n")
	if !anyContains(vs, "无法解析") {
		t.Fatalf("当前无法解析应报违规，得到 %v", vs)
	}
}

func TestCompareCoverage(t *testing.T) {
	mk := func(v string) string { return "X ?= 1\nMIN_COVERAGE ?= " + v + "\n" }
	cases := []struct {
		name, old, cur string
		want           int
	}{
		{"下调", mk("98"), mk("97.5"), 1},
		{"不变", mk("98"), mk("98"), 0},
		{"上调", mk("98"), mk("99"), 0},
		{"删除", mk("98"), "X ?= 1\n", 1},
		{"改成变量", mk("98"), "MIN_COVERAGE ?= $(X)\n", 1},
		{"基线没有不检查", "X ?= 1\n", mk("10"), 0},
	}
	for _, c := range cases {
		got := compareCoverage(c.old, c.cur)
		t.Logf("%s → %v", c.name, got)
		if len(got) != c.want {
			t.Fatalf("%s：期望 %d 条违规，得到 %v", c.name, c.want, got)
		}
		if c.want > 0 && (got[0].Rule != ruleCoverage || got[0].Override != overrideGovernance) {
			t.Fatalf("%s：违规标识错误 %+v", c.name, got[0])
		}
	}
}

func TestDiffLines(t *testing.T) {
	diff := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n-\told()\n-   \n+\tnew()\n+\tt.Skip(\"x\")\n"
	if got := removedLines(diff); len(got) != 1 || got[0] != (diffLine{1, "\told()"}) {
		t.Fatalf("removedLines 应只取非空删除行且跳过 --- 头，得到 %v", got)
	}
	// 多个 hunk：行号按各自 hunk 头的旧文件起点累加；空白删除行也占行号
	multi := "@@ -3,2 +3,0 @@\n-a\n-\n@@ -10 +8 @@\n-b\n+c\n@@ -20,0 +18,1 @@\n+d\n"
	got := removedLines(multi)
	t.Logf("%v", got)
	if len(got) != 2 || got[0] != (diffLine{3, "a"}) || got[1] != (diffLine{10, "b"}) {
		t.Fatalf("多 hunk 行号错误，得到 %v", got)
	}
	if got := addedLines(diff); len(got) != 2 || got[1] != "\tt.Skip(\"x\")" {
		t.Fatalf("addedLines 应跳过 +++ 头，得到 %q", got)
	}
}

func TestCharFuncRanges(t *testing.T) {
	src := "package internal\n\n" + // 1-2
		"func TestDoRequest_A(t *testing.T) {\n\tassert(1)\n}\n\n" + // 3-5
		"func TestReqA_B(t *testing.T) {\n\tassert(2)\n}\n\n" + // 7-9
		"func (r *rec) TestDoRequest_方法不算() {}\n\n" + // 11
		"func TestDoRawRequest_C(t *testing.T) { assert(3) }\n" // 13
	got := charFuncRanges(src, testCharRule)
	t.Logf("%v", got)
	want := []lineRange{{3, 5}, {13, 13}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("期望 %v，得到 %v", want, got)
	}
	if !inRanges(4, got) || inRanges(8, got) || inRanges(6, got) {
		t.Fatalf("inRanges 判定错误：%v", got)
	}
	if got := charFuncRanges("package x\nfunc {", testCharRule); len(got) != 1 || got[0].From != 1 || !inRanges(1<<30, got) {
		t.Fatalf("解析失败应按整文件处理，得到 %v", got)
	}
	if got := charFuncRanges(src, charRule{path: testCharPath}); len(got) != 1 || !inRanges(8, got) {
		t.Fatalf("未配置函数正则应按整文件处理，得到 %v", got)
	}
}

func TestParseCharRule(t *testing.T) {
	cases := []struct {
		name, src   string
		wantPath    string
		wantFuncRe  string // 空表示 funcRe 为 nil
		wantErrText string // 非空表示期望报错且文案含此片段
	}{
		{name: "空配置不检查", src: ""},
		{name: "只配文件按整文件", src: "characterization:\n  file: a_test.go\n", wantPath: "a_test.go"},
		{name: "文件加函数正则", src: "characterization:\n  file: a_test.go\n  func_pattern: ^TestA_\n", wantPath: "a_test.go", wantFuncRe: "^TestA_"},
		{name: "YAML 非法", src: "characterization: [", wantErrText: "无法解析"},
		{name: "正则非法", src: "characterization:\n  file: a_test.go\n  func_pattern: \"(\"\n", wantErrText: "func_pattern 非法"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rule, err := parseCharRule(c.src)
			if c.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErrText) {
					t.Fatalf("期望含 %q 的错误，得到 %v", c.wantErrText, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错：%v", err)
			}
			if rule.path != c.wantPath {
				t.Fatalf("path 期望 %q，得到 %q", c.wantPath, rule.path)
			}
			gotRe := ""
			if rule.funcRe != nil {
				gotRe = rule.funcRe.String()
			}
			if gotRe != c.wantFuncRe {
				t.Fatalf("funcRe 期望 %q，得到 %q", c.wantFuncRe, gotRe)
			}
		})
	}
}

func TestSkipViolations(t *testing.T) {
	lines := []string{`t.Skip("flaky")`, `b.Skipf("x %d", 1)`, `f.SkipNow()`, `// 这里不 t.Skipped()`, `ts.Skip(1)`, `skip := true`}
	vs := skipViolations(lines)
	t.Logf("%v", vs)
	if len(vs) != 3 {
		t.Fatalf("期望 t/b/f 三种 Skip 调用各报一条，得到 %d：%v", len(vs), vs)
	}
}

func TestFilterOverridden(t *testing.T) {
	vs := []violation{
		{ruleCoverage, "c", overrideGovernance},
		{ruleChar, "h", overrideBehavior},
	}
	cases := []struct {
		name, text string
		want       int
	}{
		{"无标记", "feat: x", 2},
		{"治理豁免带理由", "治理豁免: 删掉死代码后分母变化", 1},
		{"治理豁免全角冒号", "治理豁免：理由", 1},
		{"治理豁免无理由不生效", "治理豁免: ", 2},
		{"行为变更", "行为变更：重试次数 3→5", 1},
		{"两者都有", "行为变更\n治理豁免: x", 0},
	}
	for _, c := range cases {
		if got := filterOverridden(vs, c.text); len(got) != c.want {
			t.Fatalf("%s：期望剩 %d 条，得到 %v", c.name, c.want, got)
		}
	}
}

// TestRunGovernance_真实仓库端到端 在临时 git 仓库里走一遍：基线提交 → 工作区放宽 → 检查失败 → 提交说明豁免 → 通过。
func TestRunGovernance_真实仓库端到端(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(p, s string) {
		t.Helper()
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "commit.gpgsign", "false")
	write(makefilePath, "MIN_COVERAGE ?= 98\n")
	write(golangciPath, baseGolangci)
	write(configPath, testConfig)
	write(testCharPath, charSrc("3", "1"))
	git("add", ".")
	git("commit", "-q", "-m", "base")
	t.Chdir(dir)
	t.Setenv("GOVERNANCE_BASE", "")
	t.Setenv("GOVERNANCE_OVERRIDE_TEXT", "")

	if code := runGovernance([]string{"--base", "HEAD"}); code != 0 {
		t.Fatalf("无改动应通过，退出码 %d", code)
	}

	// 只改非 char 用例、在 char 用例里只新增行：都不算删改
	write(testCharPath, charSrc("3", "2")+"\nfunc TestDoRequest_新增(t *testing.T) {}\n")
	if vs := charViolations(dir, "HEAD"); len(vs) != 0 {
		t.Fatalf("非 char 用例改动与新增 char 用例不应报违规，得到 %v", vs)
	}

	// 工作区里改配置（换文件 / 清空）不能绕过：规则取自基线
	write(configPath, "characterization:\n  file: other_test.go\n")
	write(testCharPath, charSrc("4", "1"))
	if vs := charViolations(dir, "HEAD"); len(vs) != 1 {
		t.Fatalf("工作区改配置不应绕过 char 检查，得到 %v", vs)
	}
	write(configPath, testConfig)

	write(makefilePath, "MIN_COVERAGE ?= 90\n")
	write(testCharPath, charSrc("5", "1"))
	write("pkg/new_test.go", "package pkg\nfunc TestB(t *testing.T) { t.Skip(\"later\") }\n")
	vs := collectViolations(dir, "HEAD", true)
	t.Logf("%v", vs)
	for _, rule := range []string{ruleCoverage, ruleChar, ruleSkip} {
		if !anyRule(vs, rule) {
			t.Fatalf("应报 %s 违规，得到 %v", rule, vs)
		}
	}
	if code := runGovernance([]string{"--base", "HEAD", "--worktree"}); code != 1 {
		t.Fatalf("有违规应退出 1，得到 %d", code)
	}

	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
	git("add", ".")
	git("commit", "-q", "-m", "chore: x\n\n治理豁免: 测试用\n行为变更：断言 3→5")
	if code := runGovernance([]string{"--base", base}); code != 0 {
		t.Fatalf("提交说明带标记应放行，退出码 %d", code)
	}
	if code := runGovernance([]string{"--base", base, "--worktree"}); code != 1 {
		t.Fatalf("--worktree 不认提交说明标记，应退出 1，得到 %d", code)
	}
	if code := runGovernance([]string{"--base", zeroSHA}); code != 0 {
		t.Fatalf("全零基线应跳过，退出码 %d", code)
	}
}

// testCharPath / testConfig / testCharRule 是测试用的 char 规则：文件里只有 TestDoRequest_ / TestDoRawRequest_ 算 char 用例。
const (
	testCharPath = "internal/request_test.go"
	testConfig   = "characterization:\n  file: " + testCharPath + "\n  func_pattern: ^Test(DoRequest|DoRawRequest)_\n"
)

var testCharRule = charRule{path: testCharPath, funcRe: regexp.MustCompile(`^Test(DoRequest|DoRawRequest)_`)}

// TestCharViolations_基线配置 覆盖基线侧配置的三种形态：没有配置不检查、配置非法报违规、只配文件按整文件检查。
func TestCharViolations_基线配置(t *testing.T) {
	cases := []struct {
		name, config string // config 为空表示基线没有 .agentguard.yml
		change       func() string
		wantN        int
	}{
		{name: "没有配置不检查", change: func() string { return charSrc("5", "1") }, wantN: 0},
		{name: "配置非法报违规", config: "characterization: [", change: func() string { return charSrc("3", "1") }, wantN: 1},
		{name: "只配文件时改普通用例也算", config: "characterization:\n  file: " + testCharPath + "\n", change: func() string { return charSrc("3", "2") }, wantN: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{testCharPath: charSrc("3", "1")}
			if c.config != "" {
				files[configPath] = c.config
			}
			initRepo(t, dir, files)
			if err := os.WriteFile(filepath.Join(dir, testCharPath), []byte(c.change()), 0o600); err != nil {
				t.Fatal(err)
			}
			vs := charViolations(dir, "HEAD")
			t.Logf("%v", vs)
			if len(vs) != c.wantN {
				t.Fatalf("期望 %d 条违规，得到 %v", c.wantN, vs)
			}
		})
	}
}

// initRepo 在 dir 建 git 仓库，写入 files 并提交为基线。
func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, s := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "commit.gpgsign", "false"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// charSrc 生成一个含 char 用例（TestDoRequest_A 断言 charVal）与普通用例（TestReqA_B 断言 otherVal）的测试文件。
func charSrc(charVal, otherVal string) string {
	return "package internal\n\nfunc TestDoRequest_A(t *testing.T) {\n\tassert(" + charVal + ")\n}\n\n" +
		"func TestReqA_B(t *testing.T) {\n\tassert(" + otherVal + ")\n}\n"
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	s, err := gitOut(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return s
}

func anyContains(vs []violation, sub string) bool {
	for _, v := range vs {
		if strings.Contains(v.Detail, sub) {
			return true
		}
	}
	return false
}

func anyRule(vs []violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}
