package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// callHook 以 stdin 调 runHook，捕获退出码、stdout、stderr。改了进程级 os.Std*，测试不得并行。
func callHook(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = inR, outW, errW
	defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
	go func() { _, _ = io.WriteString(inW, stdin); _ = inW.Close() }()
	code := runHook(args)
	_ = outW.Close()
	_ = errW.Close()
	out, _ := io.ReadAll(outR)
	errOut, _ := io.ReadAll(errR)
	t.Logf("hook %v → code=%d stdout=%s stderr=%s", args, code, out, errOut)
	return code, string(out), string(errOut)
}

func input(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHook_Claude前置裁决(t *testing.T) {
	root := newRepo(t, "feat/x")
	bash := func(cmd string) string {
		return input(t, map[string]any{"cwd": root, "tool_name": "Bash", "tool_input": map[string]any{"command": cmd}})
	}
	code, out, _ := callHook(t, bash("git push --force"), "--agent", "claude", "--event", "pre-tool")
	if code != 0 || !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, `"hookEventName":"PreToolUse"`) {
		t.Fatalf("强推应输出 deny JSON，得到 code=%d out=%s", code, out)
	}
	if code, out, _ = callHook(t, bash("go test ./..."), "--agent", "claude", "--event", "pre-tool"); code != 0 || out != "" {
		t.Fatalf("放行时应无输出，得到 code=%d out=%s", code, out)
	}

	// 门禁基础设施不弹确认框：前置放行，放宽由治理守卫在改后 / 收尾识别
	edit := input(t, map[string]any{"cwd": root, "tool_name": "Edit", "tool_input": map[string]any{"file_path": filepath.Join(root, ".golangci.yml")}})
	if _, out, _ = callHook(t, edit, "--agent", "claude", "--event", "pre-tool"); out != "" {
		t.Fatalf("改门禁配置应直接放行、不弹确认，得到 %s", out)
	}
	gitInternal := input(t, map[string]any{"cwd": root, "tool_name": "Write", "tool_input": map[string]any{"file_path": filepath.Join(root, ".git", "config")}})
	if _, out, _ = callHook(t, gitInternal, "--agent", "claude", "--event", "pre-tool"); !strings.Contains(out, `"deny"`) {
		t.Fatalf("改 .git 内部应拒绝，得到 %s", out)
	}
	if strings.Contains(out, `"ask"`) {
		t.Fatalf("任何情况都不应输出 ask，得到 %s", out)
	}

	read := input(t, map[string]any{"cwd": root, "tool_name": "Read", "tool_input": map[string]any{"file_path": filepath.Join(root, ".env")}})
	if _, out, _ = callHook(t, read, "--agent", "claude", "--event", "pre-tool"); !strings.Contains(out, `"deny"`) {
		t.Fatalf("读 .env 应拒绝，得到 %s", out)
	}
}

func TestHook_Codex前置裁决用退出码2(t *testing.T) {
	root := newRepo(t, "feat/x")
	in := input(t, map[string]any{"cwd": root, "tool_name": "Bash", "tool_input": map[string]any{"command": []any{"bash", "-lc", "git tag -d v1"}}})
	code, _, errOut := callHook(t, in, "--agent", "codex", "--event", "pre-tool")
	if code != 2 || !strings.Contains(errOut, "tag") {
		t.Fatalf("Codex 拦截应退出 2 并写 stderr，得到 %d %q", code, errOut)
	}
	patch := "*** Begin Patch\n*** Update File: .github/workflows/ci.yml\n@@\n-a\n+b\n*** End Patch"
	in = input(t, map[string]any{"cwd": root, "tool_name": "apply_patch", "tool_input": map[string]any{"command": patch}})
	if code, _, _ = callHook(t, in, "--agent", "codex", "--event", "pre-tool"); code != 0 {
		t.Fatalf("补丁改 CI 配置应放行，得到 %d", code)
	}
	patch = "*** Begin Patch\n*** Add File: .env\n+A=1\n*** End Patch"
	in = input(t, map[string]any{"cwd": root, "tool_name": "apply_patch", "tool_input": map[string]any{"command": patch}})
	if code, _, _ = callHook(t, in, "--agent", "codex", "--event", "pre-tool"); code != 2 {
		t.Fatalf("补丁写 .env 应拦截，得到 %d", code)
	}
	patch = "*** Begin Patch\n*** Add File: httpx/new.go\n+package httpx\n*** End Patch"
	in = input(t, map[string]any{"cwd": root, "tool_name": "apply_patch", "tool_input": map[string]any{"command": patch}})
	if code, _, _ = callHook(t, in, "--agent", "codex", "--event", "pre-tool"); code != 0 {
		t.Fatalf("普通补丁应放行，得到 %d", code)
	}
}

func TestHook_Cursor前置裁决(t *testing.T) {
	root := newRepo(t, "feat/x")
	_, out, _ := callHook(t, input(t, map[string]any{"cwd": root, "command": "git commit --no-verify -m x"}), "--agent", "cursor", "--event", "pre-shell")
	if !strings.Contains(out, `"permission":"deny"`) || !strings.Contains(out, "agent_message") {
		t.Fatalf("Cursor 应输出 deny 与 agent_message，得到 %s", out)
	}
	if _, out, _ = callHook(t, input(t, map[string]any{"cwd": root, "command": "ls"}), "--agent", "cursor", "--event", "pre-shell"); strings.TrimSpace(out) != `{"permission":"allow"}` {
		t.Fatalf("Cursor 放行应显式 allow，得到 %s", out)
	}
	if _, out, _ = callHook(t, input(t, map[string]any{"cwd": root, "file_path": filepath.Join(root, ".env")}), "--agent", "cursor", "--event", "pre-read"); !strings.Contains(out, `"deny"`) {
		t.Fatalf("Cursor 读 .env 应拒绝，得到 %s", out)
	}
	edit := input(t, map[string]any{"cwd": root, "tool_name": "Write", "tool_input": map[string]any{"file_path": ".githooks/pre-push"}})
	if _, out, _ = callHook(t, edit, "--agent", "cursor", "--event", "pre-tool"); strings.TrimSpace(out) != `{"permission":"allow"}` {
		t.Fatalf("Cursor 改钩子应放行，得到 %s", out)
	}
	secret := input(t, map[string]any{"cwd": root, "tool_name": "Write", "tool_input": map[string]any{"file_path": "id_rsa"}})
	if _, out, _ = callHook(t, secret, "--agent", "cursor", "--event", "pre-tool"); !strings.Contains(out, `"deny"`) {
		t.Fatalf("Cursor 写私钥应拒绝，得到 %s", out)
	}
}

func TestHook_编辑后gofmt与vet反馈(t *testing.T) {
	root := newRepo(t, "feat/x")
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.24\n")
	f := write(t, root, "a/a.go", "package a\nfunc  A( ) int {return 1}\n")
	in := input(t, map[string]any{"cwd": root, "tool_name": "Write", "tool_input": map[string]any{"file_path": f}})
	if _, out, _ := callHook(t, in, "--agent", "claude", "--event", "post-tool"); out != "" {
		t.Fatalf("格式化后 vet 通过应无反馈，得到 %s", out)
	}
	if b, _ := os.ReadFile(f); !strings.Contains(string(b), "func A() int { return 1 }") {
		t.Fatalf("应已 gofmt -w，得到 %s", b)
	}
	write(t, root, "a/a.go", "package a\nimport \"fmt\"\nfunc A() string { return fmt.Sprintf(\"%d\") }\n")
	_, out, _ := callHook(t, in, "--agent", "claude", "--event", "post-tool")
	if !strings.Contains(out, `"decision":"block"`) || !strings.Contains(out, "go vet") {
		t.Fatalf("vet 失败应反馈 block，得到 %s", out)
	}
	cur := input(t, map[string]any{"cwd": root, "file_path": f})
	if _, out, _ = callHook(t, cur, "--agent", "cursor", "--event", "post-tool"); !strings.Contains(out, "additional_context") {
		t.Fatalf("Cursor 反馈应走 additional_context，得到 %s", out)
	}
}

func TestHook_收尾检查与防死循环(t *testing.T) {
	root := newRepo(t, "feat/x")
	stop := func(agent string) string {
		_, out, _ := callHook(t, input(t, map[string]any{"cwd": root}), "--agent", agent, "--event", "stop")
		return out
	}
	if out := stop("claude"); out != "" {
		t.Fatalf("无改动应直接放行，得到 %s", out)
	}
	write(t, root, "Makefile", "check:\n\t@echo 覆盖率不达标; exit 1\n")
	write(t, root, "x.go", "package x\n")
	for i := 1; i <= maxStopBlocks; i++ {
		out := stop("claude")
		if !strings.Contains(out, `"decision":"block"`) || !strings.Contains(out, "覆盖率不达标") {
			t.Fatalf("第 %d 次应 block 并带 make check 输出，得到 %s", i, out)
		}
	}
	if out := stop("claude"); !strings.Contains(out, "systemMessage") {
		t.Fatalf("超过上限应放行并提示人，得到 %s", out)
	}
	if out := stop("cursor"); !strings.Contains(out, "followup_message") {
		t.Fatalf("计数已重置，Cursor 应再次要求继续，得到 %s", out)
	}
	write(t, root, "Makefile", "check:\n\t@true\n")
	if out := stop("codex"); out != "" {
		t.Fatalf("检查通过应放行，得到 %s", out)
	}
	if n := readCount(gitPath(root, stopCounterRel)); n != 0 {
		t.Fatalf("通过后计数应清零，得到 %d", n)
	}
}

func TestHook_收尾治理违规与放行标记(t *testing.T) {
	root := newRepo(t, "feat/x")
	write(t, root, "Makefile", "MIN_COVERAGE ?= 98\ncheck:\n\t@true\n")
	if out, err := runCmd(root, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qam", "x", "--allow-empty"); err != nil {
		t.Fatal(out)
	}
	if out, err := runCmd(root, "git", "add", "Makefile"); err != nil {
		t.Fatal(out)
	}
	if out, err := runCmd(root, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "mk"); err != nil {
		t.Fatal(out)
	}
	write(t, root, "Makefile", "MIN_COVERAGE ?= 90\ncheck:\n\t@true\n")
	_, out, _ := callHook(t, input(t, map[string]any{"cwd": root}), "--agent", "claude", "--event", "stop")
	if !strings.Contains(out, "MIN_COVERAGE") {
		t.Fatalf("下调覆盖率应在收尾被拦，得到 %s", out)
	}
	if err := os.WriteFile(gitPath(root, allowMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, out, _ = callHook(t, input(t, map[string]any{"cwd": root}), "--agent", "claude", "--event", "stop"); out != "" {
		t.Fatalf("有放行标记时治理违规不拦，得到 %s", out)
	}
}

func TestHook_参数与仓库外(t *testing.T) {
	if code, _, _ := callHook(t, "{}", "--agent", "claude", "--event", "nope"); code != 1 {
		t.Fatalf("未知事件应退出 1，得到 %d", code)
	}
	if code, _, _ := callHook(t, "{}", "--bogus"); code != 1 {
		t.Fatalf("未知参数应退出 1，得到 %d", code)
	}
	outside := input(t, map[string]any{"cwd": t.TempDir(), "tool_name": "Bash", "tool_input": map[string]any{"command": "git push -f"}})
	if code, out, _ := callHook(t, outside, "--agent", "claude", "--event", "pre-tool"); code != 0 || out != "" {
		t.Fatalf("仓库外一律放行，得到 %d %s", code, out)
	}
	if code, _, _ := callHook(t, "not json", "--agent", "codex", "--event", "pre-tool"); code != 0 {
		t.Fatalf("输入无法解析应放行，得到 %d", code)
	}
}

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
