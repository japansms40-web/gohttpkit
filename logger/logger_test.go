package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
)

// packageDefault 是包加载时放好的缺省 logger；TestMain 换成静默输出之前先记下，供缺省输出的用例断言。
var packageDefault *slog.Logger

func TestMain(m *testing.M) {
	packageDefault = current.Load()
	current.Store(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// useHandler 把 h 装成当前输出，用例结束时还原。
func useHandler(t *testing.T, h slog.Handler) {
	t.Helper()
	prev := current.Load()
	current.Store(slog.New(h))
	t.Cleanup(func() { current.Store(prev) })
}

func captureJSON(t *testing.T, opts *slog.HandlerOptions) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	useHandler(t, slog.NewJSONHandler(buf, opts))
	return buf
}

// errHandler 的 Handle 永远返回错误，用来验证门面吞掉输出错误、以及未启用时不进 Handle。
type errHandler struct{ enabled bool }

func (e errHandler) Enabled(context.Context, slog.Level) bool  { return e.enabled }
func (e errHandler) Handle(context.Context, slog.Record) error { return io.ErrUnexpectedEOF }
func (e errHandler) WithAttrs([]slog.Attr) slog.Handler        { return e }
func (e errHandler) WithGroup(string) slog.Handler             { return e }

func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("解析日志 JSON 失败: %v\n原文: %s", err, buf.String())
	}
	return m
}

// assertLevelMsgN 断言 buf 最后一行日志是 level 级、msg 为 the-msg、带 n=7，供四级透传用例共用。
func assertLevelMsgN(t *testing.T, buf *bytes.Buffer, level string) {
	t.Helper()
	m := lastLine(t, buf)
	t.Logf("level=%v msg=%v n=%v", m["level"], m["msg"], m["n"])
	if m["level"] != level {
		t.Errorf("level = %v, want %v", m["level"], level)
	}
	if m["msg"] != "the-msg" {
		t.Errorf("msg = %v, want the-msg", m["msg"])
	}
	if m["n"] != float64(7) {
		t.Errorf("attr n = %v, want 7", m["n"])
	}
}

func TestCurrent_包加载后即有缺省输出(t *testing.T) {
	if packageDefault == nil {
		t.Fatal("包加载后 current 应已放好缺省 logger")
	}
	h := packageDefault.Handler()
	t.Logf("default handler=%T", h)
	if _, ok := h.(*slog.JSONHandler); !ok {
		t.Fatalf("缺省输出应是 JSON 处理器，实际 %T", h)
	}
	if !h.Enabled(t.Context(), slog.LevelInfo) || h.Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("缺省输出应是 info 级：放行 info、拦下 debug")
	}
}

func TestLog_ctx里的trace_id自动写出(t *testing.T) {
	buf := captureJSON(t, nil)

	Info(WithTraceID(t.Context(), "lg"), "with-trace")
	m := lastLine(t, buf)
	t.Logf("msg=%v trace_id=%v", m["msg"], m["trace_id"])
	if m["msg"] != "with-trace" {
		t.Fatalf("当前输出未生效: %v", m)
	}
	if m["trace_id"] != "lg" {
		t.Fatalf("门面日志未自动带 trace_id: %v", m)
	}
}

func TestLog_只有WithAttrs无trace(t *testing.T) {
	buf := captureJSON(t, nil)
	Info(WithAttrs(t.Context(), slog.String("account_id", "u9")), "attrs-only")
	m := lastLine(t, buf)
	t.Logf("attrs-only=%v", m)
	if m["account_id"] != "u9" {
		t.Fatalf("WithAttrs 未写出: %v", m)
	}
	if m["trace_id"] != nil || m["span_id"] != nil {
		t.Fatalf("无 trace/span 的 ctx 不应冒出这两个键: %v", m)
	}
}

func TestLog_Handle错误被吞掉(t *testing.T) {
	useHandler(t, errHandler{enabled: true})

	Info(t.Context(), "ignored-1")
	Error(WithAttrs(WithTraceID(t.Context(), "t"), slog.String("a", "1")), "ignored-2")
	t.Logf("Handle 返回 error 后未向上传播")
}

func TestLog_未启用时直接返回(t *testing.T) {
	useHandler(t, errHandler{enabled: false})
	Info(t.Context(), "should-skip")
	t.Logf("Enabled=false 时不进 Handle")
}

func TestEnabled_随当前输出的级别变化且nil_ctx不panic(t *testing.T) {
	captureJSON(t, &slog.HandlerOptions{Level: slog.LevelWarn})

	var nilCtx context.Context
	cases := []struct {
		name  string
		ctx   context.Context
		level slog.Level
		want  bool
	}{
		{"低于阈值", t.Context(), slog.LevelInfo, false},
		{"等于阈值", t.Context(), slog.LevelWarn, true},
		{"高于阈值", t.Context(), slog.LevelError, true},
		{"nil ctx 回落 Background", nilCtx, slog.LevelError, true},
		{"nil ctx 低于阈值", nilCtx, slog.LevelDebug, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Enabled(c.ctx, c.level)
			t.Logf("Enabled(%v) → %v", c.level, got)
			if got != c.want {
				t.Fatalf("Enabled(%v) = %v, want %v", c.level, got, c.want)
			}
		})
	}
}

func TestLevelFunctions_四级透传msg与attr(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{Level: slog.LevelDebug})
	cases := []struct {
		name  string
		fn    func(context.Context, string, ...slog.Attr)
		level string
	}{
		{"Debug", Debug, "DEBUG"},
		{"Info", Info, "INFO"},
		{"Warn", Warn, "WARN"},
		{"Error", Error, "ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf.Reset()
			c.fn(t.Context(), "the-msg", slog.Int("n", 7))
			assertLevelMsgN(t, buf, c.level)
		})
	}
}

func TestLog_低于阈值不写出(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{Level: slog.LevelWarn})

	Debug(t.Context(), "dropped-debug")
	Info(t.Context(), "dropped-info")
	t.Logf("below warn buf=%q", buf.String())
	if buf.Len() != 0 {
		t.Fatalf("低于 warn 的日志不应输出: %s", buf.String())
	}

	Warn(t.Context(), "kept-warn")
	m := lastLine(t, buf)
	t.Logf("warn → %v", m["msg"])
	if m["msg"] != "kept-warn" {
		t.Fatalf("warn 级别应输出: %v", m)
	}
}

func TestInfo_nilContext回落Background(t *testing.T) {
	buf := captureJSON(t, nil)
	var nilCtx context.Context
	Info(nilCtx, "nil-ctx-ok")
	m := lastLine(t, buf)
	t.Logf("nil ctx → %v", m["msg"])
	if m["msg"] != "nil-ctx-ok" {
		t.Fatalf("nil ctx 应正常输出: %v", m)
	}
}

func TestErr_nil不产键非nil写出(t *testing.T) {
	buf := captureJSON(t, nil)

	Info(t.Context(), "nil errs", Err(nil), NamedErr("classify_err", nil))
	m := lastLine(t, buf)
	t.Logf("nil errs keys error=%v classify_err=%v", m["error"], m["classify_err"])
	if _, ok := m["error"]; ok {
		t.Fatalf("Err(nil) 不应输出 error 键: %v", m)
	}
	if _, ok := m["classify_err"]; ok {
		t.Fatalf("NamedErr(k, nil) 不应输出键: %v", m)
	}

	buf.Reset()
	Info(t.Context(), "real err", Err(context.DeadlineExceeded))
	m = lastLine(t, buf)
	t.Logf("Err(DeadlineExceeded) → %v", m["error"])
	if m["error"] != context.DeadlineExceeded.Error() {
		t.Fatalf("Err 输出错误: %v", m["error"])
	}
}

func TestNamedErr_空键名仍写出(t *testing.T) {
	a := NamedErr("", io.EOF)
	t.Logf("NamedErr(\"\", EOF) → key=%q val=%q", a.Key, a.Value.String())
	if a.Key != "" || a.Value.String() != io.EOF.Error() {
		t.Fatalf("空键名仍应写出值: key=%q val=%q", a.Key, a.Value.String())
	}
}

func TestNamedErr_自定义键名(t *testing.T) {
	buf := captureJSON(t, nil)
	Info(t.Context(), "named", NamedErr("classify_err", context.Canceled))
	m := lastLine(t, buf)
	t.Logf("classify_err=%v", m["classify_err"])
	if m["classify_err"] != context.Canceled.Error() {
		t.Fatalf("classify_err = %v, want %v", m["classify_err"], context.Canceled.Error())
	}
}

func TestSourceLocation_指向本测试文件(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{AddSource: true})

	Info(t.Context(), "where am i")
	m := lastLine(t, buf)
	src, _ := m["source"].(map[string]any)
	t.Logf("source=%v", src)
	if src == nil {
		t.Fatalf("缺少 source 字段: %v", m)
	}
	file, _ := src["file"].(string)
	if !strings.HasSuffix(file, "logger_test.go") {
		t.Fatalf("source.file = %q, want *logger_test.go(callers skip 偏移错误)", file)
	}
}

func TestConcurrentSetAndLog_混跑不炸(t *testing.T) {
	prev := current.Load()
	t.Cleanup(func() { current.Store(prev) })
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for range 100 {
				current.Store(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				current.Store(prev)
			}
		}()
		go func() {
			defer wg.Done()
			ctx := WithTraceID(t.Context(), "race")
			for j := range 100 {
				Info(ctx, "concurrent", slog.Int("j", j))
			}
		}()
	}
	wg.Wait()
	t.Logf("8×3 goroutine ×100 次 Set/Log 结束")
}
