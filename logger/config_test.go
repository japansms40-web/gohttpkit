package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// mustInit 调 Init 并在用例结束时 Close，保证进程级日志状态不泄漏到别的用例。
func mustInit(t *testing.T, c Config, attrs ...slog.Attr) {
	t.Helper()
	if err := Init(c, attrs...); err != nil {
		t.Fatalf("Init(%+v) 失败: %v", c, err)
	}
	t.Cleanup(Close)
}

// processLogState 是 Init / Close 会改动的进程级状态，用来断言「没变」或「已还原」。
type processLogState struct {
	installed   *slog.Logger
	slogDefault *slog.Logger
	stdOut      io.Writer
	stdFlags    int
	live        *session
}

func snapshotLogState() processLogState {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	return processLogState{installed: installed.Load(), slogDefault: slog.Default(), stdOut: stdlog.Writer(), stdFlags: stdlog.Flags(), live: live}
}

func TestDefaultConfig_单点默认值(t *testing.T) {
	c := DefaultConfig()
	t.Logf("DefaultConfig = %+v", c)
	want := Config{Level: LevelInfo, Format: FormatJSON, Output: OutputConsole, Filename: "./logs/app.log"}
	if c != want {
		t.Fatalf("DefaultConfig() = %+v, want %+v", c, want)
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

func TestInit_file写入指定路径并建目录(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "app.log")
	var console bytes.Buffer
	mustInit(t, Config{Level: LevelInfo, Format: FormatJSON, Output: OutputFile, Filename: path, Writer: &console})

	Info(t.Context(), "to-file-unique-xyz")

	data, err := os.ReadFile(path)
	t.Logf("path=%s err=%v body=%s", path, err, data)
	if err != nil {
		t.Fatalf("读日志文件失败(目录或文件未创建): %v", err)
	}
	if !strings.Contains(string(data), "to-file-unique-xyz") {
		t.Fatalf("日志未写入文件: %s", data)
	}
	if console.Len() != 0 {
		t.Fatalf("Output=file 不应写控制台: %s", console.String())
	}
}

func TestInit_启动时就创建日志文件(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eager", "app.log")
	mustInit(t, Config{Level: LevelError, Output: OutputFile, Filename: path})

	info, err := os.Stat(path)
	t.Logf("stat err=%v", err)
	if err != nil {
		t.Fatalf("Init 应当场建目录并创建日志文件，不等第一条日志: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("探测写入应是零长度，文件大小 = %d", info.Size())
	}
}

func TestInit_both同时写控制台与文件(t *testing.T) {
	path := filepath.Join(t.TempDir(), "both.log")
	var console bytes.Buffer
	mustInit(t, Config{Level: "info", Format: "console", Output: "both", Filename: path, Writer: &console})

	Info(t.Context(), "both-unique-abc")

	fileData, err := os.ReadFile(path)
	t.Logf("console=%s file=%s err=%v", console.String(), fileData, err)
	if !strings.Contains(console.String(), "both-unique-abc") {
		t.Fatalf("both 模式未写入控制台: %s", console.String())
	}
	if err != nil || !strings.Contains(string(fileData), "both-unique-abc") {
		t.Fatalf("both 模式未写入文件: err=%v data=%s", err, fileData)
	}
}

func TestInit_Writer为nil且未知Output时写stdout(t *testing.T) {
	origStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = origStdout })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	mustInit(t, Config{Level: LevelInfo, Format: FormatJSON, Output: "syslog"})
	Info(t.Context(), "unknown-output-stdout")

	os.Stdout = origStdout
	_ = w.Close()
	stdoutData, _ := io.ReadAll(r)
	t.Logf("stdout=%s", stdoutData)
	if !strings.Contains(string(stdoutData), "unknown-output-stdout") {
		t.Fatalf("Writer 为 nil、Output 未知时应回落 stdout: %s", stdoutData)
	}
}

func TestInit_空Filename回落默认路径(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	mustInit(t, Config{Level: LevelInfo, Format: FormatJSON, Output: OutputFile})

	Info(t.Context(), "default-path-msg")

	data, err := os.ReadFile(filepath.Join(dir, "logs", "app.log"))
	t.Logf("default path body=%s err=%v", data, err)
	if err != nil {
		t.Fatalf("空 Filename 应回落 ./logs/app.log: %v", err)
	}
	if !strings.Contains(string(data), "default-path-msg") {
		t.Fatalf("默认路径未写入: %s", data)
	}
}

func TestInit_落盘失败返回FileSetupError且进程日志状态不变(t *testing.T) {
	dir := t.TempDir()
	notDir := filepath.Join(dir, "afile")
	if err := os.WriteFile(notDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
	}{
		{"父路径是普通文件", filepath.Join(notDir, "sub", "app.log")},
		{"路径本身是目录", dir},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := snapshotLogState()
			for _, output := range []Output{OutputFile, OutputBoth} {
				err := Init(Config{Level: LevelInfo, Output: output, Filename: c.path})
				assertFileSetup(t, err, "open", c.path)
				var fe *FileSetupError
				if !errors.As(err, &fe) || fe.Err == nil {
					t.Fatal("应带底层 Err")
				}
			}
			if after := snapshotLogState(); after != before {
				t.Fatalf("Init 失败后进程日志状态变了:\nbefore=%+v\nafter=%+v", before, after)
			}
		})
	}
	// 路径是目录时不能交给轮转库：它会把这个目录改名成备份再新建同名文件。
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("调用方的目录应原样保留: info=%v err=%v", info, err)
	}
	var pathErr *fs.PathError
	if err := Init(Config{Output: OutputFile, Filename: dir}); !errors.As(err, &pathErr) {
		t.Fatalf("路径是目录时内层应是 *fs.PathError: %v", err)
	}
}

func TestInit_零值回落info且走JSON(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Writer: &buf})

	Debug(t.Context(), "zero-debug-should-drop")
	Info(t.Context(), "zero-info-keep")

	t.Logf("zero config body=%s", buf.String())
	if strings.Contains(buf.String(), "zero-debug-should-drop") {
		t.Fatal("空 Level 应回落 info，debug 不应写出")
	}
	if !strings.Contains(buf.String(), `"msg":"zero-info-keep"`) {
		t.Fatalf("空 Format 应按 JSON 写出 info: %s", buf.String())
	}
}

func TestInit_非法Format按JSON(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelInfo, Format: "XML", Writer: &buf})

	Info(t.Context(), "xml-as-json")

	t.Logf("illegal format body=%s", buf.String())
	if !json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("非 console 的 Format 应按 JSON: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"msg":"xml-as-json"`) {
		t.Fatalf("JSON 行缺少 msg: %s", buf.String())
	}
}

func TestInit_FormatConsole走文本(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelInfo, Format: FormatConsole, Writer: &buf})

	Info(t.Context(), "console-line")

	t.Logf("console body=%s", buf.String())
	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("FormatConsole 不应产出 JSON: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "console-line") {
		t.Fatalf("文本行缺少消息: %s", buf.String())
	}
}

func TestInit_LevelError过滤低级别(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelError, Format: FormatJSON, Writer: &buf})

	Debug(t.Context(), "drop-debug")
	Info(t.Context(), "drop-info")
	Warn(t.Context(), "drop-warn")
	Error(t.Context(), "keep-error")

	body := buf.String()
	t.Logf("error-level body=%s", body)
	for _, drop := range []string{"drop-debug", "drop-info", "drop-warn"} {
		if strings.Contains(body, drop) {
			t.Fatalf("LevelError 不应写出 %s: %s", drop, body)
		}
	}
	if !strings.Contains(body, `"msg":"keep-error"`) {
		t.Fatalf("LevelError 应写出 error: %s", body)
	}
}

func TestInit_LevelDebug放行Debug(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelDebug, Format: FormatJSON, Writer: &buf})

	Debug(t.Context(), "debug-ok")

	t.Logf("debug body=%s", buf.String())
	if !strings.Contains(buf.String(), `"msg":"debug-ok"`) {
		t.Fatalf("LevelDebug 应写出 debug: %s", buf.String())
	}
}

func TestInit_AddSource指向业务调用点(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelInfo, Format: FormatJSON, Writer: &buf})

	Info(t.Context(), "src-line")

	t.Logf("source body=%s", buf.String())
	if !strings.Contains(buf.String(), "config_test.go") {
		t.Fatalf("处理器应 AddSource 到测试调用点: %s", buf.String())
	}
}

func TestInit_固定字段出现在每一行且自动带trace_id(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelInfo, Writer: &buf}, slog.String("app", "svc"), slog.Int64("job_id", 42))

	Info(WithTraceID(t.Context(), "fixed-trace"), "first")
	Named("mod").Info("second")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	t.Logf("lines=%v", lines)
	if len(lines) != 2 {
		t.Fatalf("应有 2 行，实际 %d 行: %s", len(lines), buf.String())
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("第 %d 行不是 JSON: %v", i, err)
		}
		if m["app"] != "svc" || m["job_id"] != float64(42) {
			t.Fatalf("第 %d 行缺固定字段: %v", i, m)
		}
	}
	if !strings.Contains(lines[0], `"trace_id":"fixed-trace"`) {
		t.Fatalf("Init 安装的处理器应自动带 trace_id: %s", lines[0])
	}
}

func TestInit_已有文件追加不覆盖(t *testing.T) {
	path := filepath.Join(t.TempDir(), "append.log")
	for _, msg := range []string{"first-line", "second-line"} {
		if err := Init(Config{Level: LevelInfo, Output: OutputFile, Filename: path}); err != nil {
			t.Fatal(err)
		}
		Info(t.Context(), msg)
		Close()
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

func TestInit_新日志文件权限600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perm.log")
	mustInit(t, Config{Output: OutputFile, Filename: path})

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

func TestOpenLogFile_轮转参数传给轮转库且负数按0(t *testing.T) {
	cases := []struct {
		name         string
		in           Config
		wantSize     int
		wantBackups  int
		wantAge      int
		wantCompress bool
	}{
		{"原样传递", Config{MaxSize: 200, MaxBackups: 20, MaxAge: 7, Compress: true}, 200, 20, 7, true},
		{"零值交给轮转库缺省", Config{}, 0, 0, 0, false},
		{"负数按0", Config{MaxSize: -1, MaxBackups: -2, MaxAge: -3}, 0, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.in.Filename = filepath.Join(t.TempDir(), "rotate.log")
			file, err := openLogFile(c.in)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = file.Close() })
			t.Logf("Filename=%s MaxSize=%d MaxBackups=%d MaxAge=%d Compress=%v", file.Filename, file.MaxSize, file.MaxBackups, file.MaxAge, file.Compress)
			if file.Filename != c.in.Filename || file.MaxSize != c.wantSize || file.MaxBackups != c.wantBackups || file.MaxAge != c.wantAge || file.Compress != c.wantCompress {
				t.Fatalf("轮转参数与期望不符")
			}
		})
	}
}

func TestInit_超过MaxSize时切出历史文件(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")
	mustInit(t, Config{Level: LevelInfo, Output: OutputFile, Filename: path, MaxSize: 1})

	chunk := strings.Repeat("x", 64*1024)
	for range 20 {
		Info(t.Context(), "big", slog.String("payload", chunk))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("写入约 1.3MB 后目录里有 %d 个文件", len(entries))
	if len(entries) < 2 {
		t.Fatalf("单文件上限 1MB，写入约 1.3MB 后应切出历史文件，目录里只有 %d 个文件", len(entries))
	}
}

func TestInit_标准库log与slog默认logger并入同一通道(t *testing.T) {
	var buf bytes.Buffer
	mustInit(t, Config{Level: LevelInfo, Writer: &buf}, slog.String("app", "svc"))

	// 与 net/http 在 idle 连接上读到脏字节时打的那行同构：它走标准库全局 log，不经过本门面。
	stdlog.Printf("Unsolicited response received on idle HTTP channel starting with %q", "\x05\x03")
	slog.Default().Info("third-party-slog-line")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	t.Logf("lines=%v", lines)
	if len(lines) != 2 {
		t.Fatalf("标准库 log 与 slog 默认 logger 各应产生 1 行，实际 %d 行: %s", len(lines), buf.String())
	}
	wantMsgs := []string{"Unsolicited response received on idle HTTP channel", "third-party-slog-line"}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("第 %d 行不是 JSON（裸文本会破坏结构化采集）: %v\n%s", i, err, line)
		}
		msg, _ := m["msg"].(string)
		if !strings.Contains(msg, wantMsgs[i]) || m["level"] != "INFO" || m["app"] != "svc" {
			t.Fatalf("第 %d 行 = %v", i, m)
		}
	}
}

func TestClose_还原进程日志状态(t *testing.T) {
	before := snapshotLogState()
	path := filepath.Join(t.TempDir(), "close.log")
	if err := Init(Config{Level: LevelInfo, Output: OutputFile, Filename: path}); err != nil {
		t.Fatal(err)
	}
	during := snapshotLogState()
	if during.installed == before.installed || during.slogDefault == before.slogDefault || during.live == nil {
		t.Fatalf("Init 应替换当前输出与 slog 默认 logger: %+v", during)
	}

	Close()

	if after := snapshotLogState(); after != before {
		t.Fatalf("Close 应把进程日志状态还原到 Init 之前:\nbefore=%+v\nafter=%+v", before, after)
	}
	Info(t.Context(), "after-close-should-not-reach-file")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("file after close=%q", data)
	if strings.Contains(string(data), "after-close-should-not-reach-file") {
		t.Fatalf("Close 之后不应再写入那次 Init 的日志文件: %s", data)
	}
}

func TestClose_嵌套后进先出且多调无害(t *testing.T) {
	before := snapshotLogState()
	var outer, inner bytes.Buffer
	if err := Init(Config{Level: LevelInfo, Writer: &outer}); err != nil {
		t.Fatal(err)
	}
	if err := Init(Config{Level: LevelInfo, Writer: &inner}); err != nil {
		t.Fatal(err)
	}

	Info(t.Context(), "to-inner")
	Close()
	Info(t.Context(), "to-outer")
	Close()
	Info(t.Context(), "to-default")
	Close()
	Close()

	t.Logf("outer=%s inner=%s", outer.String(), inner.String())
	if !strings.Contains(inner.String(), "to-inner") || strings.Contains(inner.String(), "to-outer") {
		t.Fatalf("内层应只收到 to-inner: %s", inner.String())
	}
	if !strings.Contains(outer.String(), "to-outer") || strings.Contains(outer.String(), "to-inner") || strings.Contains(outer.String(), "to-default") {
		t.Fatalf("外层应只收到 to-outer: %s", outer.String())
	}
	if after := snapshotLogState(); after != before {
		t.Fatalf("两次 Close 之后应回到最初状态，多余的 Close 不应再改动:\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestInitClose_并发与打日志混跑不炸(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 50 {
				if err := Init(Config{Level: LevelDebug, Writer: io.Discard}); err != nil {
					t.Error(err)
					return
				}
				Close()
			}
		}()
		go func() {
			defer wg.Done()
			ctx := WithTraceID(t.Context(), "race")
			for j := range 100 {
				Info(ctx, "concurrent", slog.Int("j", j))
				_ = Enabled(ctx, slog.LevelDebug)
			}
		}()
	}
	wg.Wait()
	if snapshotLogState().live != nil {
		t.Fatal("每次 Init 都配了一次 Close，跑完后不应还有未撤销的 Init")
	}
}
