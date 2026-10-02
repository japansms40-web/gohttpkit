package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureOutput 在 fn 执行期间接管 os.Stdout / os.Stderr，返回二者内容。
// 会改进程级全局变量，用到它的用例不得 t.Parallel。
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	read := func(f **os.File) func() string {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		orig := *f
		*f = w
		done := make(chan string)
		go func() {
			b, _ := io.ReadAll(r)
			done <- string(b)
		}()
		return func() string {
			*f = orig
			_ = w.Close()
			return <-done
		}
	}
	stopOut, stopErr := read(&os.Stdout), read(&os.Stderr)
	fn()
	return stopOut(), stopErr()
}

func TestRun_无参数与未知子命令返回1并打印用法(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {""}} {
		var code int
		_, errOut := captureOutput(t, func() { code = run(args) })
		t.Logf("args=%q → code=%d stderr=%q", args, code, errOut)
		if code != 1 || !strings.Contains(errOut, "用法：agentguard") || !strings.Contains(errOut, "check-test-layout") || !strings.Contains(errOut, "check-pkg-doc") {
			t.Fatalf("args=%q 应退出 1 并打印含全部子命令的用法，得到 code=%d stderr=%q", args, code, errOut)
		}
	}
}

func TestRun_check子命令在合规仓库返回0(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"pkg/a.go":      "package pkg\nfunc A() int { return 1 }\n",
		"pkg/a_test.go": "package pkg\n",
		"pkg/doc.go":    "// Package pkg 示例。\n//\n// 文件结构：\n//\n//\t├── a.go    入口\n//\t└── doc.go  包文档\npackage pkg\n",
	})
	t.Chdir(dir)
	for sub, okMsg := range map[string]string{
		"check-errors":      "生产代码错误构造符合规范",
		"check-test-layout": "测试文件与源文件一一对应",
		"check-pkg-doc":     "每个 Go 包有 doc.go",
	} {
		var code int
		out, errOut := captureOutput(t, func() { code = run([]string{sub}) })
		t.Logf("%s → code=%d stdout=%q stderr=%q", sub, code, out, errOut)
		if code != 0 || !strings.Contains(out, okMsg) || errOut != "" {
			t.Fatalf("%s 合规时应退出 0 且只在 stdout 报通过，得到 code=%d stdout=%q stderr=%q", sub, code, out, errOut)
		}
	}
}

func TestRun_check子命令有违规返回1并逐条打到stderr(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"pkg/a.go":           "package pkg\nimport \"errors\"\nvar E = errors.New(\"x\")\nfunc A() {}\n",
		"pkg/a_test.go":      "package pkg\n",
		"pkg/a_more_test.go": "package pkg\n",
	})
	t.Chdir(dir)
	for sub, want := range map[string]string{
		"check-errors":      "pkg/a.go:3: 禁止 errors.New",
		"check-test-layout": "pkg/a_more_test.go: 测试文件须与同目录源文件同名",
		"check-pkg-doc":     "pkg/doc.go: 缺少 doc.go",
	} {
		var code int
		out, errOut := captureOutput(t, func() { code = run([]string{sub}) })
		t.Logf("%s → code=%d stdout=%q stderr=%q", sub, code, out, errOut)
		if code != 1 || !strings.Contains(errOut, want) || out != "" {
			t.Fatalf("%s 有违规应退出 1 并在 stderr 列出 %q，得到 code=%d stdout=%q stderr=%q", sub, want, code, out, errOut)
		}
	}
}

func TestRun_check子命令不在仓库内返回1(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, sub := range []string{"check-errors", "check-test-layout", "check-pkg-doc"} {
		var code int
		_, errOut := captureOutput(t, func() { code = run([]string{sub}) })
		t.Logf("%s → code=%d stderr=%q", sub, code, errOut)
		if code != 1 || !strings.Contains(errOut, sub+"：不在 Git 仓库内") {
			t.Fatalf("%s 仓库外应退出 1 并说明原因，得到 code=%d stderr=%q", sub, code, errOut)
		}
	}
}

func TestRun_check子命令扫描失败返回1(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"pkg/a.go": "package pkg\nfunc A( {\n"})
	t.Chdir(dir)
	for _, sub := range []string{"check-errors", "check-test-layout", "check-pkg-doc"} {
		var code int
		_, errOut := captureOutput(t, func() { code = run([]string{sub}) })
		t.Logf("%s → code=%d stderr=%q", sub, code, errOut)
		if code != 1 || !strings.Contains(errOut, sub+"：检查失败") {
			t.Fatalf("%s 源码解析失败必须阻断，得到 code=%d stderr=%q", sub, code, errOut)
		}
	}
}

func TestRun_governance与hook路由到各自的参数解析(t *testing.T) {
	for _, sub := range []string{"governance", "hook"} {
		var code int
		_, errOut := captureOutput(t, func() { code = run([]string{sub, "--no-such-flag"}) })
		t.Logf("%s → code=%d stderr=%q", sub, code, errOut)
		if code != 1 || !strings.Contains(errOut, "flag provided but not defined: -no-such-flag") || strings.Contains(errOut, "用法：agentguard") {
			t.Fatalf("%s 应交给子命令自己的 FlagSet 解析，得到 code=%d stderr=%q", sub, code, errOut)
		}
	}
}

func TestRun_checkStyle路由到scanStyle(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"pkg/a.go": "package pkg\nimport \"sync\"\nvar mu sync.Mutex\nfunc A() { mu.Lock(); mu.Unlock() }\n",
	})
	t.Chdir(dir)
	var code int
	out, errOut := captureOutput(t, func() { code = run([]string{"check-style"}) })
	t.Logf("code=%d stdout=%q stderr=%q", code, out, errOut)
	if code != 1 || !strings.Contains(errOut, "pkg/a.go:4: lock-defer") {
		t.Fatalf("有违规应退出 1 并列出，得到 code=%d stderr=%q", code, errOut)
	}
}
