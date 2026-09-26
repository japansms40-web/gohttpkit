package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestNamed_带module字段(t *testing.T) {
	buf := captureJSON(t, nil)

	Named("insexecutor").Info("hello", slog.String("k", "v"))
	m := lastLine(t, buf)
	t.Logf("line=%v", m)
	if m[FieldModule] != "insexecutor" {
		t.Fatalf("module = %v, want insexecutor", m[FieldModule])
	}
	if m["msg"] != "hello" || m["k"] != "v" {
		t.Fatalf("msg/attr 未透传: %v", m)
	}
}

func TestNamed_空名不产module键(t *testing.T) {
	buf := captureJSON(t, nil)

	Named("").Info("no-module")
	m := lastLine(t, buf)
	t.Logf("line=%v", m)
	if _, ok := m[FieldModule]; ok {
		t.Fatalf("Named(\"\") 不应输出 module 键: %v", m)
	}
}

func TestLoggerLevels_四级透传msg与attr(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := Named("m")
	cases := []struct {
		name  string
		fn    func(string, ...slog.Attr)
		level string
	}{
		{"Debug", l.Debug, "DEBUG"},
		{"Info", l.Info, "INFO"},
		{"Warn", l.Warn, "WARN"},
		{"Error", l.Error, "ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf.Reset()
			c.fn("the-msg", slog.Int("n", 7))
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

func TestLogger_低于阈值不写出(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{Level: slog.LevelWarn})
	l := Named("m")

	l.Debug("dropped-debug")
	l.Info("dropped-info")
	t.Logf("below warn buf=%q", buf.String())
	if buf.Len() != 0 {
		t.Fatalf("低于 warn 的日志不应输出: %s", buf.String())
	}

	l.Warn("kept-warn")
	if m := lastLine(t, buf); m["msg"] != "kept-warn" {
		t.Fatalf("warn 级别应输出: %v", m)
	}
}

func TestLoggerWith_字段累积且顺序固定(t *testing.T) {
	buf := captureJSON(t, nil)

	Named("mod").With(slog.String("a", "1")).With(slog.String("b", "2")).Info("x", slog.String("c", "3"))
	line := buf.String()
	t.Logf("raw=%s", line)
	idx := func(key string) int { return strings.Index(line, `"`+key+`":`) }
	im, ia, ib, ic := idx(FieldModule), idx("a"), idx("b"), idx("c")
	if im < 0 || ia < 0 || ib < 0 || ic < 0 {
		t.Fatalf("字段缺失: module=%d a=%d b=%d c=%d", im, ia, ib, ic)
	}
	if im >= ia || ia >= ib || ib >= ic {
		t.Fatalf("字段顺序应为 module→a→b→c，实际 %d %d %d %d", im, ia, ib, ic)
	}
}

func TestLoggerWith_不改父实例且兄弟互不串(t *testing.T) {
	buf := captureJSON(t, nil)

	// 父切片留出多余容量：若 With 直接 append，两个子实例会写进同一底层数组相互覆盖。
	attrs := make([]slog.Attr, 1, 8)
	attrs[0] = slog.String("p", "1")
	parent := &Logger{attrs: attrs}
	left := parent.With(slog.String("side", "left"))
	right := parent.With(slog.String("side", "right"))

	left.Info("l")
	if m := lastLine(t, buf); m["side"] != "left" {
		t.Fatalf("left 子实例 side = %v, want left（被兄弟覆盖）", m["side"])
	}
	right.Info("r")
	if m := lastLine(t, buf); m["side"] != "right" {
		t.Fatalf("right 子实例 side = %v, want right", m["side"])
	}
	parent.Info("p")
	m := lastLine(t, buf)
	t.Logf("parent=%v", m)
	if _, ok := m["side"]; ok {
		t.Fatalf("父实例不应带子实例字段: %v", m)
	}
	if m["p"] != "1" {
		t.Fatalf("父实例字段丢失: %v", m)
	}
}

func TestLoggerWith_空参返回原实例(t *testing.T) {
	l := Named("m")
	if got := l.With(); got != l {
		t.Fatalf("With() 无参应返回原实例，got %p want %p", got, l)
	}
}

func TestLogger_nil与零值可用(t *testing.T) {
	buf := captureJSON(t, nil)

	var nilLogger *Logger
	nilLogger.Info("nil-ok")
	if m := lastLine(t, buf); m["msg"] != "nil-ok" {
		t.Fatalf("nil 接收者应正常输出: %v", m)
	}
	nilLogger.With(slog.String("k", "v")).Info("nil-with")
	if m := lastLine(t, buf); m["k"] != "v" {
		t.Fatalf("nil 接收者 With 应带字段: %v", m)
	}
	var zero Logger
	zero.Warn("zero-ok")
	m := lastLine(t, buf)
	t.Logf("zero=%v", m)
	if m["msg"] != "zero-ok" {
		t.Fatalf("零值 Logger 应正常输出: %v", m)
	}
}

func TestLogger_不带trace与span(t *testing.T) {
	buf := captureJSON(t, nil)

	Named("m").Info("plain")
	m := lastLine(t, buf)
	t.Logf("line=%v", m)
	if _, ok := m[FieldTraceID]; ok {
		t.Fatalf("不传 ctx 不应有 trace_id: %v", m)
	}
	if _, ok := m[FieldSpanID]; ok {
		t.Fatalf("不传 ctx 不应有 span_id: %v", m)
	}
}

func TestLogger_调用时才解析当前handler(t *testing.T) {
	l := Named("early") // 模拟包级 var：先于 SetHandler 创建
	buf := captureJSON(t, nil)

	l.Info("after-set")
	if m := lastLine(t, buf); m["msg"] != "after-set" {
		t.Fatalf("先 Named 后 SetHandler，日志应进新 handler: %v", m)
	}

	prev := fallback.Load()
	fb := &bytes.Buffer{}
	fallback.Store(slog.New(wrapCtxHandler(slog.NewJSONHandler(fb, nil))))
	t.Cleanup(func() { fallback.Store(prev) })
	SetLogger(nil)
	l.Info("back-to-fallback")
	m := lastLine(t, fb)
	t.Logf("fallback=%v", m)
	if m["msg"] != "back-to-fallback" {
		t.Fatalf("SetLogger(nil) 后应回到 fallback: %v", m)
	}
}

func TestLoggerSource_指向本测试文件(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{AddSource: true})

	Named("m").Info("where am i")
	m := lastLine(t, buf)
	src, _ := m["source"].(map[string]any)
	t.Logf("source=%v", src)
	file, _ := src["file"].(string)
	if !strings.HasSuffix(file, "named_test.go") {
		t.Fatalf("source.file = %q, want *named_test.go(callers skip 偏移错误)", file)
	}
}

func TestLogger_并发With与Set混跑(t *testing.T) {
	t.Cleanup(func() { SetLogger(nil) })
	base := Named("race")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					SetHandler(slog.NewJSONHandler(&bytes.Buffer{}, nil))
				} else {
					SetLogger(nil)
				}
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				base.With(slog.Int("j", j)).Info("concurrent")
			}
		}()
	}
	wg.Wait()
	t.Logf("8×2 goroutine ×100 次 With/Info 与 Set 混跑结束")
}
