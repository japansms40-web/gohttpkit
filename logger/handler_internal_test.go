package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// TestWrapCtxHandlerIdempotent 已包装的 handler 不再二次包装(防 SetLogger(Default()) 之类的双层注入)
// 角度: #7 契约不变式 —— wrapCtxHandler 文档承诺已包装则原样返回
func TestWrapCtxHandlerIdempotent(t *testing.T) {
	inner := slog.NewJSONHandler(io.Discard, nil)

	once := wrapCtxHandler(inner)
	if _, ok := once.(*ctxHandler); !ok {
		t.Fatal("首次包装应返回 *ctxHandler")
	}

	twice := wrapCtxHandler(once)
	if once != twice {
		t.Fatal("已包装的 handler 应原样返回,不应二次包装")
	}
}

// TestNoDoubleTraceInjection 端到端验证防双层注入:重新注入当前 logger 后 trace_id 只出现一次
// 角度: #7 契约不变式(配合 TestWrapCtxHandlerIdempotent 的行为级断言)
func TestNoDoubleTraceInjection(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	// Default() 返回的已是 *ctxHandler 包装的 logger,再次注入不得叠加一层
	SetLogger(Default())

	Info(WithTraceID(context.Background(), "solo"), "msg")
	if n := strings.Count(buf.String(), "trace_id"); n != 1 {
		t.Fatalf("trace_id 应只出现一次,实际 %d 次: %s", n, buf.String())
	}
}

// TestHandlerWithAttrsDerive 经 slog.Logger.With 派生的 handler:WithAttrs 字段写出,且仍自动注入 ctx 字段
// 角度: #7 契约不变式 —— 派生 handler 必须保持 ctx 注入能力
func TestHandlerWithAttrsDerive(t *testing.T) {
	buf := &bytes.Buffer{}
	l := slog.New(wrapCtxHandler(slog.NewJSONHandler(buf, nil)))

	derived := l.With(slog.String("svc", "ig")) // 触发 ctxHandler.WithAttrs

	ctx := WithAttrs(WithTraceID(context.Background(), "tid-x"), slog.String("account_id", "u1"))
	derived.InfoContext(ctx, "hi", slog.String("k", "v"))

	m := lastLine(t, buf)
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

// TestHandlerWithGroupDerive 经 slog.Logger.WithGroup 派生:调用点 attr 落入分组,ctx 注入仍生效
// 角度: #7 契约不变式 —— WithGroup 派生保持 ctx 注入能力
func TestHandlerWithGroupDerive(t *testing.T) {
	buf := &bytes.Buffer{}
	l := slog.New(wrapCtxHandler(slog.NewJSONHandler(buf, nil)))

	g := l.WithGroup("grp") // 触发 ctxHandler.WithGroup

	ctx := WithTraceID(context.Background(), "tid-g")
	g.InfoContext(ctx, "hi", slog.String("k", "v"))

	m := lastLine(t, buf)
	grp, _ := m["grp"].(map[string]any)
	if grp == nil || grp["k"] != "v" {
		t.Fatalf("WithGroup 未将调用点 attr 归入分组: %v", m)
	}
	// trace_id 仍被注入(分组激活后 Handle 追加,落点在组内属当前实现行为,这里只校验存在性)
	if !strings.Contains(buf.String(), `"trace_id":"tid-g"`) {
		t.Fatalf("派生分组 handler 未注入 trace_id: %s", buf.String())
	}
}

// errHandler 永远在 Handle 阶段返回错误,用于验证 log() 吞掉错误不向上抛
type errHandler struct{ enabled bool }

func (e errHandler) Enabled(context.Context, slog.Level) bool  { return e.enabled }
func (e errHandler) Handle(context.Context, slog.Record) error { return errors.New("boom") }
func (e errHandler) WithAttrs([]slog.Attr) slog.Handler        { return e }
func (e errHandler) WithGroup(string) slog.Handler             { return e }

// TestHandleErrorSwallowed inner handler 返回 error 时 log() 不 panic、不传播(error 被显式忽略)
// 角度: #4 错误传播 + #3 错误路径 —— 覆盖 log() 中 `_ = Handle()` 的容错语义
func TestHandleErrorSwallowed(t *testing.T) {
	SetHandler(errHandler{enabled: true})
	t.Cleanup(func() { SetLogger(nil) })

	// 无 trace(走 Handle 直传分支)与有 trace(走 Clone 分支)均不得 panic
	Info(context.Background(), "ignored-1")
	Error(WithAttrs(WithTraceID(context.Background(), "t"), slog.String("a", "1")), "ignored-2")
}
