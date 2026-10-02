package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const sharedStart = "<!-- shared-rules:start —— 说明 -->"

func TestExtractSharedBlock_按起止标记取段内内容(t *testing.T) {
	cases := []struct {
		name, doc, want string
		ok              bool
	}{
		{"成对标记", "# T\n" + sharedStart + "\nA\nB\n" + sharedEndMarker + "\n尾", "A\nB", true},
		{"缺结束标记", sharedStart + "\nA\n", "", false},
		{"顺序颠倒", sharedEndMarker + "\nA\n" + sharedStart + "\n", "", false},
		{"没有标记", "# T\n", "", false},
	}
	for _, c := range cases {
		got, ok := extractSharedBlock(c.doc)
		t.Logf("%s → %q ok=%v", c.name, got, ok)
		if got != c.want || ok != c.ok {
			t.Fatalf("%s：期望 %q ok=%v，得到 %q ok=%v", c.name, c.want, c.ok, got, ok)
		}
	}
}

func TestCheckSharedRules_与正本一致无违规(t *testing.T) {
	good := sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n"
	if got := checkSharedRules(good); len(got) != 0 {
		t.Fatalf("与正本一致应无违规，得到 %v", got)
	}
}

func TestCheckSharedRules_漂移报一条并给出行号(t *testing.T) {
	drift := sharedStart + "\n" + strings.Replace(sharedRules, "中文", "英文", 1) + "\n" + sharedEndMarker + "\n"
	got := checkSharedRules(drift)
	t.Logf("drift → %v", got)
	if len(got) != 1 || !strings.Contains(got[0], "与 agentguard 内嵌正本不一致") || !strings.Contains(got[0], "行起不同") {
		t.Fatalf("漂移应报一条并给出行号，得到 %v", got)
	}
}

func TestCheckSharedRules_缺标记报一条(t *testing.T) {
	if got := checkSharedRules("# 无标记\n"); len(got) != 1 || !strings.Contains(got[0], "缺少成对的") {
		t.Fatalf("缺标记应报一条，得到 %v", got)
	}
}

func TestParseMDCFrontmatter_只读frontmatter里的alwaysApply和globs(t *testing.T) {
	cases := []struct {
		name, src string
		always    bool
		globs     []string
	}{
		{"alwaysApply", "---\ndescription: x\nalwaysApply: true\n---\n正文 globs: a/**\n", true, nil},
		{"逗号分隔", "---\nglobs: a/**,b/*.go\nalwaysApply: false\n---\n", false, []string{"a/**", "b/*.go"}},
		{"带引号", "---\nglobs: \"**/*_test.go\"\n---\n", false, []string{"**/*_test.go"}},
		{"没有frontmatter", "# 标题\nglobs: a/**\n", false, nil},
	}
	for _, c := range cases {
		always, globs := parseMDCFrontmatter(c.src)
		t.Logf("%s → always=%v globs=%v", c.name, always, globs)
		if always != c.always || strings.Join(globs, "|") != strings.Join(c.globs, "|") {
			t.Fatalf("%s：期望 always=%v globs=%v", c.name, c.always, c.globs)
		}
	}
}

func TestGlobRegexp_双星跨目录单星不跨(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"android/**", "android/v361/a.go", true},
		{"android/**", "web/a.go", false},
		{"**/*_test.go", "a_test.go", true},
		{"**/*_test.go", "x/y/a_test.go", true},
		{"pkg/headers*.go", "pkg/headers_a.go", true},
		{"pkg/headers*.go", "pkg/x/headers.go", false},
		{"web/*.go", "web/a/b.go", false},
		{"a?.go", "ab.go", true},
		{"a.go", "aXgo", false},
		{"中文/**", "中文/a.go", true},
	}
	for _, c := range cases {
		got := globRegexp(c.glob).MatchString(c.path)
		t.Logf("%s ~ %s → %v", c.glob, c.path, got)
		if got != c.want {
			t.Fatalf("%s ~ %s：期望 %v", c.glob, c.path, c.want)
		}
	}
}

func TestCheckCursorGlobs_失效glob与无加载条件的规则被报出(t *testing.T) {
	rules := map[string]string{
		"00-core.mdc": "---\nalwaysApply: true\n---\n",
		"dead.mdc":    "---\nglobs: dead/**\n---\n",
		"half.mdc":    "---\nglobs: a/**,b/*.go\n---\n",
		"none.mdc":    "---\ndescription: x\n---\n",
		"ok.mdc":      "---\nglobs: a/**\n---\n",
	}
	got := checkCursorGlobs(rules, []string{"a/x.go", "c.go"})
	t.Logf("→ %v", got)
	assertPrefixes(t, got, []string{
		".cursor/rules/dead.mdc: glob dead/** 匹配不到",
		".cursor/rules/half.mdc: glob b/*.go 匹配不到",
		".cursor/rules/none.mdc: 既不是 alwaysApply 也没有 globs",
	})
}

func TestScanAgentsDocs_仓库端到端(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		agentsPath:                   sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n",
		cursorRulesDir + "/dead.mdc": "---\nglobs: gone/**\n---\n",
		cursorRulesDir + "/ok.mdc":   "---\nglobs: pkg/**\n---\n",
		"pkg/a.go":                   "package pkg\n",
	})
	got, err := scanAgentsDocs(dir)
	t.Logf("→ %v err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	assertPrefixes(t, got, []string{".cursor/rules/dead.mdc: glob gone/**"})
}

func TestScanAgentsDocs_缺AGENTS返回错误(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.txt": "x"})
	_, err := scanAgentsDocs(dir)
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Path != agentsPath {
		t.Fatalf("缺 AGENTS.md 应返回 *os.PathError，得到 %v", err)
	}
}

func TestScanAgentsDocs_没有cursor规则目录只查共享段(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{agentsPath: sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n"})
	got, err := scanAgentsDocs(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("没有 .cursor/rules 应无违规，得到 %v err=%v", got, err)
	}
}
