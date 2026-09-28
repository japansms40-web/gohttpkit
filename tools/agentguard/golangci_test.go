package main

import (
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
