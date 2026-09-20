package logger

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestWrapCtxHandler_已包装不再套一层(t *testing.T) {
	inner := slog.NewJSONHandler(io.Discard, nil)

	once := wrapCtxHandler(inner)
	t.Logf("once type=%T", once)
	if _, ok := once.(*ctxHandler); !ok {
		t.Fatal("首次包装应返回 *ctxHandler")
	}

	twice := wrapCtxHandler(once)
	t.Logf("same pointer=%v", once == twice)
	if once != twice {
		t.Fatal("已包装的 handler 应原样返回,不应二次包装")
	}
}

func TestSetLogger_Default不再双写trace_id(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	SetLogger(Default())

	Info(WithTraceID(context.Background(), "solo"), "msg")
	n := strings.Count(buf.String(), "trace_id")
	t.Logf("trace_id count=%d body=%s", n, buf.String())
	if n != 1 {
		t.Fatalf("trace_id 应只出现一次,实际 %d 次: %s", n, buf.String())
	}
}

func TestHandlerWithAttrs_派生仍注入ctx字段(t *testing.T) {
	buf := &bytes.Buffer{}
	l := slog.New(wrapCtxHandler(slog.NewJSONHandler(buf, nil)))

	derived := l.With(slog.String("svc", "ig"))

	ctx := WithAttrs(WithTraceID(context.Background(), "tid-x"), slog.String("account_id", "u1"))
	derived.InfoContext(ctx, "hi", slog.String("k", "v"))

	m := lastLine(t, buf)
	t.Logf("derived = %v", m)
	if m["svc"] != "ig" {
		t.Errorf("WithAttrs 注入的 svc 字段丢失: %v", m)
	}
	if m["trace_id"] != "tid-x" {
		t.Errorf("派生 handler 未自动注入 trace_id: %v", m)
	}
	if m["account_id"] != "u1" {
		t.Errorf("派生 handler 未自动注入 ctx 通用字段: %v", m)
	}
	if m["k"] != "v" {
		t.Errorf("调用点 attr 丢失: %v", m)
	}
}

func TestHandlerWithGroup_调用点进组且仍有trace(t *testing.T) {
	buf := &bytes.Buffer{}
	l := slog.New(wrapCtxHandler(slog.NewJSONHandler(buf, nil)))

	g := l.WithGroup("grp")

	ctx := WithTraceID(context.Background(), "tid-g")
	g.InfoContext(ctx, "hi", slog.String("k", "v"))

	m := lastLine(t, buf)
	t.Logf("grouped = %v", m)
	grp, _ := m["grp"].(map[string]any)
	if grp == nil || grp["k"] != "v" {
		t.Fatalf("WithGroup 未将调用点 attr 归入分组: %v", m)
	}
	if !strings.Contains(buf.String(), `"trace_id":"tid-g"`) {
		t.Fatalf("派生分组 handler 未注入 trace_id: %s", buf.String())
	}
}

func TestHandle_只有WithAttrs无trace(t *testing.T) {
	buf := captureJSON(t, nil)
	Info(WithAttrs(context.Background(), slog.String("account_id", "u9")), "attrs-only")
	m := lastLine(t, buf)
	t.Logf("attrs-only=%v", m)
	if m["account_id"] != "u9" {
		t.Fatalf("WithAttrs 未写出: %v", m)
	}
	if m["trace_id"] != nil || m["span_id"] != nil {
		t.Fatalf("无 trace/span 的 ctx 不应冒出这两个键: %v", m)
	}
}

type errHandler struct{ enabled bool }

func (e errHandler) Enabled(context.Context, slog.Level) bool  { return e.enabled }
func (e errHandler) Handle(context.Context, slog.Record) error { return io.ErrUnexpectedEOF }
func (e errHandler) WithAttrs([]slog.Attr) slog.Handler        { return e }
func (e errHandler) WithGroup(string) slog.Handler             { return e }

func TestLog_inner错误被吞掉(t *testing.T) {
	SetHandler(errHandler{enabled: true})
	t.Cleanup(func() { SetLogger(nil) })

	Info(context.Background(), "ignored-1")
	Error(WithAttrs(WithTraceID(context.Background(), "t"), slog.String("a", "1")), "ignored-2")
	t.Logf("Handle 返回 error 后未向上传播")
}

func TestLog_inner关闭时直接返回(t *testing.T) {
	SetHandler(errHandler{enabled: false})
	t.Cleanup(func() { SetLogger(nil) })
	Info(context.Background(), "should-skip")
	t.Logf("Enabled=false 时不进 Handle")
}
