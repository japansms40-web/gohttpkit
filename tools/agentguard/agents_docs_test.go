package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sharedStart = "<!-- shared-rules:start —— 说明 -->"

func TestExtractSharedBlock_按起止标记取段内内容(t *testing.T) {
	cases := []struct {
		name, doc, want string
		startIdx        int
		ok              bool
	}{
		{"成对标记", "# T\n" + sharedStart + "\nA\nB\n" + sharedEndMarker + "\n尾", "A\nB", 1, true},
		{"标记行带前导空白", "  \t" + sharedStart + "\nA\n \t" + sharedEndMarker + "\n", "A", 0, true},
		{"缺结束标记", sharedStart + "\nA\n", "", 0, false},
		{"顺序颠倒", sharedEndMarker + "\nA\n" + sharedStart + "\n", "", 0, false},
		{"没有标记", "# T\n", "", 0, false},
	}
	for _, c := range cases {
		got, idx, ok := extractSharedBlock(c.doc)
		t.Logf("%s → %q idx=%d ok=%v", c.name, got, idx, ok)
		if got != c.want || ok != c.ok || (ok && idx != c.startIdx) {
			t.Fatalf("%s：期望 %q idx=%d ok=%v，得到 %q idx=%d ok=%v", c.name, c.want, c.startIdx, c.ok, got, idx, ok)
		}
	}
}

func TestCheckSharedRules_与正本一致无违规(t *testing.T) {
	good := sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n"
	if got := checkSharedRules(good); len(got) != 0 {
		t.Fatalf("与正本一致应无违规，得到 %v", got)
	}
}

func TestCheckSharedRules_漂移报一条并给出文件行号与段内行号(t *testing.T) {
	// 标记在第 3 行，段内有 1 个前导空行，漂移在正本第 1 行 → 文件第 3+1+1=5 行。
	drift := "# T\n\n" + sharedStart + "\n\n" + strings.Replace(sharedRules, firstLine(sharedRules), "被改动的首行", 1) + "\n" + sharedEndMarker + "\n"
	got := checkSharedRules(drift)
	t.Logf("drift → %v", got)
	if len(got) != 1 || !strings.Contains(got[0], "与 agentguard 内嵌正本不一致") || !strings.Contains(got[0], "AGENTS.md 第 5 行（段内第 1 行）起不同") {
		t.Fatalf("漂移应报一条并给出文件行号与段内行号，得到 %v", got)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func TestCheckSharedRules_没有标记按数量报错(t *testing.T) {
	got := checkSharedRules("# 无标记\n")
	if len(got) != 1 || !strings.Contains(got[0], "shared-rules 标记须恰好各一个（start 0 个，end 0 个）") {
		t.Fatalf("缺标记应报数量，得到 %v", got)
	}
}

func TestCheckSharedRules_重复标记按数量报错(t *testing.T) {
	doc := sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n" + sharedStart + "\n" + sharedEndMarker + "\n"
	got := checkSharedRules(doc)
	if len(got) != 1 || !strings.Contains(got[0], "（start 2 个，end 2 个）") {
		t.Fatalf("重复标记应报数量，得到 %v", got)
	}
}

func TestCheckSharedRules_标记顺序颠倒报缺少成对(t *testing.T) {
	got := checkSharedRules(sharedEndMarker + "\n" + sharedRules + "\n" + sharedStart + "\n")
	if len(got) != 1 || !strings.Contains(got[0], "缺少成对的") {
		t.Fatalf("顺序颠倒应报缺少成对，得到 %v", got)
	}
}

func TestCheckSharedRules_CRLF文档与带前导空白的标记视为一致(t *testing.T) {
	doc := "  " + sharedStart + "\n" + sharedRules + "\n\t" + sharedEndMarker + "\n"
	doc = strings.ReplaceAll(doc, "\n", "\r\n")
	if got := checkSharedRules(doc); len(got) != 0 {
		t.Fatalf("CRLF 与前导空白不应造成违规，得到 %v", got)
	}
}

func TestParseMDCFrontmatter_只读frontmatter里的alwaysApply和globs(t *testing.T) {
	cases := []struct {
		name, src string
		always    bool
		globs     []string
		ok        bool
	}{
		{"alwaysApply", "---\ndescription: x\nalwaysApply: true\n---\n正文 globs: a/**\n", true, nil, true},
		{"逗号分隔", "---\nglobs: a/**,b/*.go\nalwaysApply: false\n---\n", false, []string{"a/**", "b/*.go"}, true},
		{"带引号", "---\nglobs: \"**/*_test.go\"\n---\n", false, []string{"**/*_test.go"}, true},
		{"没有frontmatter", "# 标题\nglobs: a/**\n", false, nil, true},
		{"BOM开头", "\ufeff---\nalwaysApply: true\n---\n", true, nil, true},
		{"alwaysApply大小写引号与行内注释", "---\nalwaysApply: \"TRUE\" # 说明\n---\n", true, nil, true},
		{"alwaysApply非true", "---\nalwaysApply: yes\n---\n", false, nil, true},
		{"流列表", "---\nglobs: [\"a/**\", 'b/**']\n---\n", false, []string{"a/**", "b/**"}, true},
		{"块列表", "---\nglobs:\n  - \"a/**\"\n  - b/*.go\nalwaysApply: false\n---\n", false, []string{"a/**", "b/*.go"}, true},
		{"块列表中间有空行", "---\nglobs:\n  - a/**\n\n  - b/**\n---\n", false, []string{"a/**", "b/**"}, true},
		{"单行行内注释", "---\nglobs: a/** # 注释\n---\n", false, []string{"a/**"}, true},
		{"块列表行内注释", "---\nglobs:\n  - a/** # 注释\n---\n", false, []string{"a/**"}, true},
		{"流列表行内注释", "---\nglobs: [\"a/**\", b/**] # 注释\n---\n", false, []string{"a/**", "b/**"}, true},
		{"花括号整行保留不拆分", "---\nglobs: a/{x,y}/**\n---\n", false, []string{"a/{x,y}/**"}, true},
		{"未闭合", "---\nglobs: a/**\n", false, nil, false},
	}
	for _, c := range cases {
		always, globs, ok := parseMDCFrontmatter(c.src)
		t.Logf("%s → always=%v globs=%v ok=%v", c.name, always, globs, ok)
		if always != c.always || ok != c.ok || strings.Join(globs, "|") != strings.Join(c.globs, "|") {
			t.Fatalf("%s：期望 always=%v globs=%v ok=%v", c.name, c.always, c.globs, c.ok)
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

func TestCheckCursorGlobs_失效glob被报出而无globs的规则合法(t *testing.T) {
	rules := map[string]string{
		"00-core.mdc": "---\nalwaysApply: true\n---\n",
		"dead.mdc":    "---\nglobs: dead/**\n---\n",
		"half.mdc":    "---\nglobs: a/**,b/*.go\n---\n",
		"none.mdc":    "---\ndescription: x\n---\n",
		"plain.mdc":   "# 没有 frontmatter\n",
		"ok.mdc":      "---\nglobs: a/**\n---\n",
	}
	got := checkCursorGlobs(rules, []string{"a/x.go", "c.go"})
	t.Logf("→ %v", got)
	assertPrefixes(t, got, []string{
		".cursor/rules/dead.mdc: glob dead/** 匹配不到",
		".cursor/rules/half.mdc: glob b/*.go 匹配不到",
	})
}

func TestCheckCursorGlobs_frontmatter未闭合单独报错(t *testing.T) {
	got := checkCursorGlobs(map[string]string{"open.mdc": "---\nglobs: a/**\n"}, []string{"a/x.go"})
	assertPrefixes(t, got, []string{".cursor/rules/open.mdc: frontmatter 未闭合"})
}

func TestCheckCursorGlobs_不支持的glob语法显式报出(t *testing.T) {
	rules := map[string]string{
		"brace.mdc": "---\nglobs: a/{x,y}/**\n---\n",
		"class.mdc": "---\nglobs: a/[xy].go\n---\n",
		"lead.mdc":  "---\nglobs: /a/**\n---\n",
		"trail.mdc": "---\nglobs: a/\n---\n",
		"flow.mdc":  "---\nglobs: [\"b/{c,d}\"]\n---\n",
		"fine.mdc":  "---\nglobs: a/**\n---\n",
	}
	got := checkCursorGlobs(rules, []string{"a/x/y.go", "a/x.go", "b/c"})
	t.Logf("→ %v", got)
	assertPrefixes(t, got, []string{
		".cursor/rules/brace.mdc: agentguard 不支持该 glob 语法：a/{x,y}/**（支持 ** * ? 与字面量）",
		".cursor/rules/class.mdc: agentguard 不支持该 glob 语法：a/[xy].go（支持 ** * ? 与字面量）",
		".cursor/rules/flow.mdc: agentguard 不支持该 glob 语法：b/{c,d}（支持 ** * ? 与字面量）",
		".cursor/rules/lead.mdc: agentguard 不支持该 glob 语法：/a/**（支持 ** * ? 与字面量）",
		".cursor/rules/trail.mdc: agentguard 不支持该 glob 语法：a/（支持 ** * ? 与字面量）",
	})
	for _, g := range got {
		if strings.Contains(g, "匹配不到") {
			t.Fatalf("不支持的语法不应报匹配不到：%s", g)
		}
	}
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

func TestScanAgentsDocs_只看已跟踪文件不看未跟踪文件(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		agentsPath:                  sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n",
		cursorRulesDir + "/new.mdc": "---\nglobs: fresh/**\n---\n",
	})
	if err := os.MkdirAll(filepath.Join(dir, "fresh"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fresh", "a.go"), []byte("package fresh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := scanAgentsDocs(dir)
	t.Logf("未跟踪 → %v err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	assertPrefixes(t, got, []string{".cursor/rules/new.mdc: glob fresh/** 匹配不到"})

	mustGit(t, dir, "add", "fresh/a.go")
	got, err = scanAgentsDocs(dir)
	t.Logf("已暂存 → %v err=%v", got, err)
	if err != nil || len(got) != 0 {
		t.Fatalf("已暂存文件应算已跟踪，得到 %v err=%v", got, err)
	}
}

func TestScanAgentsDocs_非git目录列文件失败返回PathError(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		agentsPath:                 sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n",
		cursorRulesDir + "/ok.mdc": "---\nglobs: a/**\n---\n",
	})
	_, err := scanAgentsDocs(dir)
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "git ls-files" || pe.Path != dir {
		t.Fatalf("非 git 目录应返回 Op=git ls-files 的 *os.PathError，得到 %v", err)
	}
}

func TestScanAgentsDocs_规则目录是普通文件时读目录失败(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		agentsPath:     sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n",
		cursorRulesDir: "我是文件",
	})
	_, err := scanAgentsDocs(dir)
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "readdir" || pe.Path != cursorRulesDir {
		t.Fatalf("规则目录是文件应返回 Op=readdir 的 *os.PathError，得到 %v", err)
	}
}

func TestScanAgentsDocs_mdc是悬空符号链接时读取失败(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{agentsPath: sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n"})
	if err := os.MkdirAll(filepath.Join(dir, cursorRulesDir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "不存在"), filepath.Join(dir, cursorRulesDir, "x.mdc")); err != nil {
		t.Fatal(err)
	}
	_, err := scanAgentsDocs(dir)
	var pe *os.PathError
	if !errors.As(err, &pe) || pe.Op != "read" || pe.Path != cursorRulesDir+"/x.mdc" {
		t.Fatalf("悬空链接应返回 Op=read 的 *os.PathError，得到 %v", err)
	}
}

func TestScanAgentsDocs_子目录与非mdc文件被跳过(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		agentsPath:                      sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n",
		cursorRulesDir + "/notes.md":    "---\nglobs: gone/**\n---\n",
		cursorRulesDir + "/sub/bad.mdc": "---\nglobs: gone/**\n---\n",
		cursorRulesDir + "/ok.mdc":      "---\nglobs: pkg/**\n---\n",
		"pkg/a.go":                      "package pkg\n",
	})
	if err := os.Mkdir(filepath.Join(dir, cursorRulesDir, "dir.mdc"), 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := scanAgentsDocs(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("子目录与非 .mdc 应被跳过，得到 %v err=%v", got, err)
	}
}

func writeTree(t *testing.T, dir string, files map[string]string) {
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
}

func TestScanAgentsDocs_没有cursor规则目录只查共享段(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{agentsPath: sharedStart + "\n" + sharedRules + "\n" + sharedEndMarker + "\n"})
	got, err := scanAgentsDocs(dir)
	if err != nil || len(got) != 0 {
		t.Fatalf("没有 .cursor/rules 应无违规，得到 %v err=%v", got, err)
	}
}
