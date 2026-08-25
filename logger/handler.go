package logger

import (
	"context"
	"log/slog"
)

// ctxHandler 包装任意 slog.Handler,把 ctx 中的 trace_id 与 WithAttrs 通用字段自动附加到每条日志。
// 默认 handler 与 SetLogger/SetHandler 注入的 handler 都会被包装 —— 调用方无需自行处理 trace 注入。
type ctxHandler struct {
	inner slog.Handler
}

// wrapCtxHandler 包装 handler;已包装过则原样返回,防止 SetLogger(Default()) 之类的双层注入
func wrapCtxHandler(h slog.Handler) slog.Handler {
	if _, ok := h.(*ctxHandler); ok {
		return h
	}
	return &ctxHandler{inner: h}
}

func (h *ctxHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	tid := TraceIDFromContext(ctx)
	sid := SpanIDFromContext(ctx)
	extra := attrsFromContext(ctx)
	if tid == "" && sid == "" && len(extra) == 0 {
		return h.inner.Handle(ctx, r)
	}
	r2 := r.Clone() // slog Handler 规范:转发前修改 Record 必须 Clone
	if tid != "" {
		r2.AddAttrs(slog.String("trace_id", tid))
	}
	if sid != "" {
		r2.AddAttrs(slog.String("span_id", sid))
	}
	r2.AddAttrs(extra...)
	return h.inner.Handle(ctx, r2)
}

func (h *ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ctxHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ctxHandler) WithGroup(name string) slog.Handler {
	return &ctxHandler{inner: h.inner.WithGroup(name)}
}
