package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommitMessage_标题与正文规则(t *testing.T) {
	scopes := map[string]bool{"httpx": true, "docs": true}
	cases := []struct {
		name, msg string
		want      []string // 每条违规须含的片段；nil 表示合规
	}{
		{"合规fix带测试行", "fix(httpx): 修重试\n\n- 背景\n\n测试：make check 通过\n", nil},
		{"测试行用半角冒号", "feat(httpx): 加功能\n\n测试:go test ./httpx/ 通过\n", nil},
		{"docs不要求测试行", "docs: 改文档\n", nil},
		{"多scope逗号分隔", "refactor(httpx,docs): 收拢\n\n测试：未运行（纯移动）\n", nil},
		{"缺测试行", "refactor(httpx): 改名\n\n- 只改名\n", []string{"缺「测试："}},
		{"测试行无内容", "perf(httpx): 提速\n\n测试：\n", []string{"缺「测试："}},
		{"scope不在白名单", "test(tests): 补用例\n", []string{"scope 不在白名单：tests"}},
		{"type非法", "feature(httpx): x\n", []string{"标题不符合"}},
		{"会话尾注", "docs(docs): x\n\nCo-Authored-By: A <a@b>\n", []string{"禁止会话尾注：Co-Authored-By"}},
		{"空说明", "# 只有注释\n\n", []string{"提交说明为空"}},
		{"剪刀线后的diff不算正文", "fix(httpx): x\n\n测试：通过\n" + scissorsLine + "\nCo-Authored-By: x\n", nil},
		{"注释行忽略", "# 注释\nfix(httpx): x\n# 注释\n\n测试：通过\n", nil},
		{"工具生成提交放行", "Revert \"fix(x): y\"\n", nil},
	}
	for _, c := range cases {
		got := checkCommitMessage(c.msg, scopes)
		t.Logf("%s → %v", c.name, got)
		if len(got) != len(c.want) {
			t.Fatalf("%s：期望 %d 条违规，得到 %v", c.name, len(c.want), got)
		}
		for i, w := range c.want {
			if !strings.Contains(got[i], w) {
				t.Fatalf("%s：第 %d 条应含 %q，得到 %q", c.name, i, w, got[i])
			}
		}
	}
}

func TestCheckCommitMessage_scope白名单为nil时不校验scope(t *testing.T) {
	got := checkCommitMessage("test(anything): x\n", nil)
	t.Logf("→ %v", got)
	if len(got) != 0 {
		t.Fatalf("未配置 scope 白名单时任何 scope 都放行，得到 %v", got)
	}
}

func TestAllowedScopes_配置加Go目录且跳过testdata(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		configPath:              "commit:\n  scopes: [docs]\n",
		"pkg/sub/a.go":          "package sub\n",
		"tools/agentguard/b.go": "package main\n",
		"pkg/testdata/x/y.go":   "package y\n",
	})
	cfg, err := loadStaticConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := allowedScopes(dir, cfg)
	t.Logf("scopes=%v err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"docs", "pkg/sub", "sub", "tools/agentguard", "agentguard"} {
		if !got[want] {
			t.Fatalf("应允许 %q，得到 %v", want, got)
		}
	}
	for _, deny := range []string{"pkg", "testdata", "x", "pkg/testdata/x"} {
		if got[deny] {
			t.Fatalf("不应允许 %q，得到 %v", deny, got)
		}
	}
}

func TestAllowedScopes_未配置scopes返回nil(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"pkg/a.go": "package pkg\n"})
	got, err := allowedScopes(dir, repoConfig{})
	if err != nil || got != nil {
		t.Fatalf("未配置 scopes 应返回 nil，得到 %v err=%v", got, err)
	}
}

func TestRun_checkCommitMsg文件模式(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{configPath: "commit:\n  scopes: [docs]\n"})
	t.Chdir(dir)
	write := func(msg string) string {
		p := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
		if err := os.WriteFile(p, []byte(msg), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var code int
	out, errOut := captureOutput(t, func() { code = run([]string{"check-commit-msg", write("docs: 改文档\n")}) })
	t.Logf("合规 → code=%d stdout=%q stderr=%q", code, out, errOut)
	if code != 0 || !strings.Contains(out, "提交说明符合规范") {
		t.Fatalf("合规说明应退出 0，得到 code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut = captureOutput(t, func() { code = run([]string{"check-commit-msg", write("fix(nope): x\n")}) })
	t.Logf("违规 → code=%d stdout=%q stderr=%q", code, out, errOut)
	if code != 1 || !strings.Contains(errOut, "scope 不在白名单：nope") || !strings.Contains(errOut, "缺「测试：") {
		t.Fatalf("违规说明应退出 1 并逐条列出，得到 code=%d stderr=%q", code, errOut)
	}
	_, errOut = captureOutput(t, func() { code = run([]string{"check-commit-msg", filepath.Join(dir, "missing")}) })
	if code != 1 || !strings.Contains(errOut, "读取提交说明失败") {
		t.Fatalf("文件不存在应退出 1，得到 code=%d stderr=%q", code, errOut)
	}
	_, errOut = captureOutput(t, func() { code = run([]string{"check-commit-msg"}) })
	if code != 1 || !strings.Contains(errOut, "用法：agentguard") {
		t.Fatalf("缺参数应打印用法并退出 1，得到 code=%d stderr=%q", code, errOut)
	}
}

func TestRun_checkCommitMsg区间模式(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.txt": "x"})
	base := mustGit(t, dir, "rev-parse", "HEAD")
	for _, msg := range []string{"docs: 合规\n", "fix: 缺测试行\n"} {
		cmd := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", msg)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}
	head := mustGit(t, dir, "rev-parse", "HEAD")
	t.Chdir(dir)
	var code int
	_, errOut := captureOutput(t, func() { code = run([]string{"check-commit-msg", "--range", base + ".." + head}) })
	t.Logf("区间 → code=%d stderr=%q", code, errOut)
	if code != 1 || !strings.Contains(errOut, short(head)+": fix 提交正文缺「测试：") || strings.Contains(errOut, "docs") {
		t.Fatalf("区间模式应只报违规提交并带短 SHA，得到 code=%d stderr=%q", code, errOut)
	}
	out, _ := captureOutput(t, func() { code = run([]string{"check-commit-msg", "--range", zeroSHA + ".." + head}) })
	if code != 0 || !strings.Contains(out, "提交说明符合规范") {
		t.Fatalf("起点为全零（新分支首推）应跳过，得到 code=%d stdout=%q", code, out)
	}
	_, errOut = captureOutput(t, func() { code = run([]string{"check-commit-msg", "--range", "no-dots"}) })
	if code != 1 || !strings.Contains(errOut, "check-commit-msg：") {
		t.Fatalf("区间格式非法应退出 1，得到 code=%d stderr=%q", code, errOut)
	}
}
