package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func restoreFallback(t *testing.T) {
	t.Helper()
	prevFb := fallback.Load()
	t.Cleanup(func() { fallback.Store(prevFb); SetLogger(nil) })
}

func TestDefaultConfig_单点默认值(t *testing.T) {
	c := DefaultConfig()
	t.Logf("DefaultConfig = %+v", c)
	if c.Level != LevelInfo || c.Format != FormatJSON || c.Output != OutputConsole || c.FilePath != "./logs/app.log" {
		t.Fatalf("DefaultConfig() = %+v", c)
	}
}

func TestParseLevel_非法值回落info(t *testing.T) {
	cases := []struct {
		name string
		in   Level
		want slog.Level
	}{
		{"debug", LevelDebug, slog.LevelDebug},
		{"info", LevelInfo, slog.LevelInfo},
		{"warn", LevelWarn, slog.LevelWarn},
		{"error", LevelError, slog.LevelError},
		{"空串兜底", "", slog.LevelInfo},
		{"大写不当debug", "DEBUG", slog.LevelInfo},
		{"大写不当info", "INFO", slog.LevelInfo},
		{"未知值兜底", "verbose", slog.LevelInfo},
		{"带空白不trim", " warn ", slog.LevelInfo},
		{"尾部空格不trim", "info ", slog.LevelInfo},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseLevel(c.in)
			t.Logf("parseLevel(%q) → %v", c.in, got)
			if got != c.want {
				t.Errorf("parseLevel(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestSetConfig_file写入指定路径并建目录(t *testing.T) {
	restoreFallback(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "app.log")

	SetLogger(nil)
	SetConfig(Config{Level: LevelInfo, Format: FormatJSON, Output: OutputFile, FilePath: path})

	Info(t.Context(), "to-file-unique-xyz")

	data, err := os.ReadFile(path)
	t.Logf("path=%s err=%v body=%s", path, err, data)
	if err != nil {
		t.Fatalf("读日志文件失败(目录或文件未创建): %v", err)
	}
	if !strings.Contains(string(data), "to-file-unique-xyz") {
		t.Fatalf("日志未写入文件: %s", data)
	}
}

func TestSetConfig_both同时写stdout与文件(t *testing.T) {
	restoreFallback(t)
	origStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = origStdout })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	dir := t.TempDir()
	path := filepath.Join(dir, "both.log")

	SetLogger(nil)
	SetConfig(Config{Level: "info", Format: "console", Output: "both", FilePath: path})
	Info(t.Context(), "both-unique-abc")

	os.Stdout = origStdout
	_ = w.Close()
	stdoutData, _ := io.ReadAll(r)
	fileData, err := os.ReadFile(path)
	t.Logf("stdout=%s file=%s err=%v", stdoutData, fileData, err)

	if !strings.Contains(string(stdoutData), "both-unique-abc") {
		t.Fatalf("both 模式未写入 stdout: %s", stdoutData)
	}
	if err != nil || !strings.Contains(string(fileData), "both-unique-abc") {
		t.Fatalf("both 模式未写入文件: err=%v data=%s", err, fileData)
	}
}

func TestMustFileWriter_空路径回落默认文件(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	w := mustFileWriter("")
	t.Logf("writer=%T", w)
	if w == nil {
		t.Fatal("空路径应返回默认文件 writer")
	}
	if _, err := os.Stat(filepath.Join(dir, "logs", "app.log")); err != nil {
		t.Fatalf("默认路径文件未创建: %v", err)
	}
}

func TestMustFileWriter_建目录失败是FileSetupError(t *testing.T) {
	dir := t.TempDir()
	notDir := filepath.Join(dir, "afile")
	if err := os.WriteFile(notDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(notDir, "sub", "app.log")

	defer func() {
		r := recover()
		t.Logf("recover=%v (%T)", r, r)
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic 值不是 error: %T %v", r, r)
		}
		assertFileSetup(t, err, "mkdir", bad)
	}()
	mustFileWriter(bad)
}

func TestMustFileWriter_打开失败是FileSetupError(t *testing.T) {
	dir := t.TempDir()

	defer func() {
		r := recover()
		t.Logf("recover=%v (%T)", r, r)
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic 值不是 error: %T %v", r, r)
		}
		assertFileSetup(t, err, "open", dir)
		var fe *FileSetupError
		if !errors.As(err, &fe) || fe.Err == nil {
			t.Fatal("应带底层 Err")
		}
	}()
	mustFileWriter(dir)
}

func TestSetConfig_零值回落info且走JSON(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.log")

	SetLogger(nil)
	SetConfig(Config{Output: OutputFile, FilePath: path})
	Debug(t.Context(), "zero-debug-should-drop")
	Info(t.Context(), "zero-info-keep")

	data, err := os.ReadFile(path)
	t.Logf("zero config body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "zero-debug-should-drop") {
		t.Fatal("空 Level 应回落 info，debug 不应写出")
	}
	if !strings.Contains(string(data), `"msg":"zero-info-keep"`) {
		t.Fatalf("空 Format 应按 JSON 写出 info: %s", data)
	}
}

func TestSetConfig_非法Format按JSON(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "fmt.log")

	SetLogger(nil)
	SetConfig(Config{Level: LevelInfo, Format: "XML", Output: OutputFile, FilePath: path})
	Info(t.Context(), "xml-as-json")

	data, err := os.ReadFile(path)
	t.Logf("illegal format body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(bytes.TrimSpace(data)) {
		t.Fatalf("非 console 的 Format 应按 JSON: %s", data)
	}
	if !strings.Contains(string(data), `"msg":"xml-as-json"`) {
		t.Fatalf("JSON 行缺少 msg: %s", data)
	}
}

func TestSetConfig_FormatConsole走文本(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")

	SetLogger(nil)
	SetConfig(Config{Level: LevelInfo, Format: FormatConsole, Output: OutputFile, FilePath: path})
	Info(t.Context(), "console-line")

	data, err := os.ReadFile(path)
	t.Logf("console body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid(bytes.TrimSpace(data)) {
		t.Fatalf("FormatConsole 不应产出 JSON: %s", data)
	}
	if !strings.Contains(string(data), "console-line") {
		t.Fatalf("文本行缺少消息: %s", data)
	}
}

func TestSetConfig_未知Output当console写stdout(t *testing.T) {
	restoreFallback(t)
	origStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = origStdout })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	SetLogger(nil)
	SetConfig(Config{Level: LevelInfo, Format: FormatJSON, Output: "syslog"})
	Info(t.Context(), "unknown-output-stdout")

	os.Stdout = origStdout
	_ = w.Close()
	stdoutData, _ := io.ReadAll(r)
	t.Logf("stdout=%s", stdoutData)
	if !strings.Contains(string(stdoutData), "unknown-output-stdout") {
		t.Fatalf("未知 Output 应回落 stdout: %s", stdoutData)
	}
}

func TestSetConfig_LevelError过滤低级别(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "err-only.log")

	SetLogger(nil)
	SetConfig(Config{Level: LevelError, Format: FormatJSON, Output: OutputFile, FilePath: path})
	Debug(t.Context(), "drop-debug")
	Info(t.Context(), "drop-info")
	Warn(t.Context(), "drop-warn")
	Error(t.Context(), "keep-error")

	data, err := os.ReadFile(path)
	t.Logf("error-level body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, drop := range []string{"drop-debug", "drop-info", "drop-warn"} {
		if strings.Contains(body, drop) {
			t.Fatalf("LevelError 不应写出 %s: %s", drop, body)
		}
	}
	if !strings.Contains(body, `"msg":"keep-error"`) {
		t.Fatalf("LevelError 应写出 error: %s", body)
	}
}

func TestSetConfig_LevelDebug放行Debug(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")

	SetLogger(nil)
	SetConfig(Config{Level: LevelDebug, Format: FormatJSON, Output: OutputFile, FilePath: path})
	Debug(t.Context(), "debug-ok")

	data, err := os.ReadFile(path)
	t.Logf("debug body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"msg":"debug-ok"`) {
		t.Fatalf("LevelDebug 应写出 debug: %s", data)
	}
}

func TestSetConfig_空FilePath回落默认路径(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	SetLogger(nil)
	SetConfig(Config{Level: LevelInfo, Format: FormatJSON, Output: OutputFile})
	Info(t.Context(), "default-path-msg")

	data, err := os.ReadFile(filepath.Join(dir, "logs", "app.log"))
	t.Logf("default path body=%s err=%v", data, err)
	if err != nil {
		t.Fatalf("空 FilePath 应回落 ./logs/app.log: %v", err)
	}
	if !strings.Contains(string(data), "default-path-msg") {
		t.Fatalf("默认路径未写入: %s", data)
	}
}

func TestMustFileWriter_追加不覆盖(t *testing.T) {
	path := filepath.Join(t.TempDir(), "append.log")
	w1 := mustFileWriter(path)
	if _, err := io.WriteString(w1, "first-line\n"); err != nil {
		t.Fatal(err)
	}
	if c, ok := w1.(io.Closer); ok {
		_ = c.Close()
	}
	w2 := mustFileWriter(path)
	if _, err := io.WriteString(w2, "second-line\n"); err != nil {
		t.Fatal(err)
	}
	if c, ok := w2.(io.Closer); ok {
		_ = c.Close()
	}

	data, err := os.ReadFile(path)
	t.Logf("append body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "first-line") || !strings.Contains(string(data), "second-line") {
		t.Fatalf("应追加而不是覆盖: %s", data)
	}
}

func TestMustFileWriter_权限600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perm.log")
	w := mustFileWriter(path)
	if c, ok := w.(io.Closer); ok {
		_ = c.Close()
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got := info.Mode().Perm()
	t.Logf("perm=%o", got)
	if got != 0o600 {
		t.Fatalf("日志文件权限 = %o, want 0600", got)
	}
}

func TestSetConfig_AddSource指向业务调用点(t *testing.T) {
	restoreFallback(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "src.log")

	SetLogger(nil)
	SetConfig(Config{Level: LevelInfo, Format: FormatJSON, Output: OutputFile, FilePath: path})
	Info(t.Context(), "src-line")

	data, err := os.ReadFile(path)
	t.Logf("source body=%s err=%v", data, err)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "config_test.go") {
		t.Fatalf("默认 handler 应 AddSource 到测试调用点: %s", data)
	}
}
