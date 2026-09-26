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

func TestMain(m *testing.M) {
	fallback.Store(slog.New(wrapCtxHandler(slog.NewJSONHandler(io.Discard, nil))))
	os.Exit(m.Run())
}

func captureJSON(t *testing.T, opts *slog.HandlerOptions) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, opts))
	t.Cleanup(func() { SetLogger(nil) })
	return buf
}

func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("解析日志 JSON 失败: %v\n原文: %s", err, buf.String())
	}
	return m
}

func TestSetLogger_注入后自动带trace_id(t *testing.T) {
	buf := &bytes.Buffer{}
	SetLogger(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })

	Info(WithTraceID(t.Context(), "lg"), "via-setlogger")
	m := lastLine(t, buf)
	t.Logf("msg=%v trace_id=%v", m["msg"], m["trace_id"])
	if m["msg"] != "via-setlogger" {
		t.Fatalf("SetLogger 注入未生效: %v", m)
	}
	if m["trace_id"] != "lg" {
		t.Fatalf("SetLogger 注入的 handler 未自动获得 trace_id: %v", m)
	}
}

func TestSetHandler_nil还原默认(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	SetHandler(nil)
	t.Logf("external after nil = %v", external.Load())
	if external.Load() != nil {
		t.Fatal("SetHandler(nil) 应清空 external")
	}

	buf.Reset()
	Info(t.Context(), "after-nil")
	t.Logf("old buf after nil = %q", buf.String())
	if buf.Len() != 0 {
		t.Fatalf("SetHandler(nil) 后不应再写入旧注入 buf: %s", buf.String())
	}
}

func TestDefault_反映当前注入(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	Default().Info("via-default")
	t.Logf("buf=%s", buf.String())
	if !strings.Contains(buf.String(), "via-default") {
		t.Fatalf("Default() 未反映已注入的 handler: %s", buf.String())
	}
}

func TestActive_惰性初始化回填fallback(t *testing.T) {
	prevExt := external.Load()
	prevFb := fallback.Load()
	t.Cleanup(func() {
		external.Store(prevExt)
		fallback.Store(prevFb)
	})

	external.Store(nil)
	fallback.Store(nil)

	got := active()
	t.Logf("active=%p fallback=%p", got, fallback.Load())
	if got == nil {
		t.Fatal("惰性初始化应返回非 nil logger")
	}
	if fallback.Load() == nil {
		t.Fatal("惰性初始化应回填 fallback")
	}
}

func TestActive_并发惰性初始化同一实例(t *testing.T) {
	prevExt := external.Load()
	prevFb := fallback.Load()
	t.Cleanup(func() {
		external.Store(prevExt)
		fallback.Store(prevFb)
	})

	external.Store(nil)

	const rounds, n = 200, 8
	for r := 0; r < rounds; r++ {
		fallback.Store(nil)

		var wg sync.WaitGroup
		start := make(chan struct{})
		got := make([]*slog.Logger, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				got[idx] = active()
			}(i)
		}
		close(start)
		wg.Wait()

		first := got[0]
		if first == nil {
			t.Fatal("惰性初始化返回 nil")
		}
		for i, l := range got {
			if l != first {
				t.Fatalf("round %d 并发惰性初始化返回不同实例: got[%d]=%p first=%p", r, i, l, first)
			}
		}
	}
	t.Logf("rounds=%d goroutines=%d 全员同一实例", rounds, n)
}

func TestInjectionPriority_SetConfig不覆盖external(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	Info(t.Context(), "via external")
	t.Logf("after SetHandler: %s", buf.String())
	if !strings.Contains(buf.String(), "via external") {
		t.Fatal("SetHandler 注入未生效")
	}

	SetConfig(Config{Level: "info", Format: "json", Output: "console"})
	buf.Reset()
	Info(t.Context(), "still external")
	t.Logf("after SetConfig: %s", buf.String())
	if !strings.Contains(buf.String(), "still external") {
		t.Fatal("SetConfig 不应覆盖已注入的 external logger")
	}

	SetLogger(nil)
	buf.Reset()
	Info(t.Context(), "to default")
	t.Logf("after SetLogger(nil) bufLen=%d", buf.Len())
	if buf.Len() != 0 {
		t.Fatal("SetLogger(nil) 后不应再写入注入的 buf")
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
			m := lastLine(t, buf)
			t.Logf("level=%v msg=%v n=%v", m["level"], m["msg"], m["n"])
			if m["level"] != c.level {
				t.Errorf("level = %v, want %v", m["level"], c.level)
			}
			if m["msg"] != "the-msg" {
				t.Errorf("msg = %v, want the-msg", m["msg"])
			}
			if m["n"] != float64(7) {
				t.Errorf("attr n = %v, want 7", m["n"])
			}
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

func TestSetLogger_nil清空external(t *testing.T) {
	SetLogger(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	t.Cleanup(func() { SetLogger(nil) })
	SetLogger(nil)
	t.Logf("external=%v", external.Load())
	if external.Load() != nil {
		t.Fatal("SetLogger(nil) 应清空 external")
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
	t.Cleanup(func() { SetLogger(nil) })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				SetHandler(slog.NewJSONHandler(&bytes.Buffer{}, nil))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				SetLogger(nil)
			}
		}()
		go func() {
			defer wg.Done()
			ctx := WithTraceID(t.Context(), "race")
			for j := 0; j < 100; j++ {
				Info(ctx, "concurrent", slog.Int("j", j))
			}
		}()
	}
	wg.Wait()
	t.Logf("8×3 goroutine ×100 次 Set/Log 结束")
}
