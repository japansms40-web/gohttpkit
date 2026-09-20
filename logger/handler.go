package logger

import (
	"context"
	"log/slog"
)

// handler.go —— slog.Handler 包装：从 ctx 注入 trace_id / span_id / WithAttrs。
// 独立成文件：这是门面与底层 slog 的接缝，和 Config 构造分开。

// ctxHandler 包装任意 slog.Handler，把 ctx 中的 trace_id 与 WithAttrs 通用字段自动附加到每条日志。
// 默认 handler 与 SetLogger/SetHandler 注入的 handler 都会被包装 —— 调用方无需自行处理 trace 注入。
// 并发：实例不可变；inner 的并发安全由被包装的 handler 负责。WithAttrs/WithGroup 返回新包装，不改原实例。
type ctxHandler struct {
	inner slog.Handler
}

// wrapCtxHandler 给任意 handler 套上 ctx 注入。
// 输入 h：已是 *ctxHandler 则原样返回同一指针，防止 SetLogger(Default()) 双层注入。
// 返回：*ctxHandler。调用方须保证 h 非 nil（SetHandler / SetLogger 已挡）。
func wrapCtxHandler(h slog.Handler) slog.Handler {
	if _, ok := h.(*ctxHandler); ok {
		return h
	}
	return &ctxHandler{inner: h}
}

// Enabled 把级别判断交给 inner。
// 输入 ctx / level：原样转发。
// 返回：inner.Enabled 的结果。
func (h *ctxHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle 在转发前把 ctx 字段写进 Record 副本。
// 输入 ctx / r：无 trace、无 span、无 WithAttrs 时不 Clone，直接交 inner。
// 返回：inner.Handle 的 error，原样上传。
// slog Handler 规范：转发前改 Record 必须 Clone，否则会污染调用方或并发写同一条。
func (h *ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	tid := TraceIDFromContext(ctx)
	sid := SpanIDFromContext(ctx)
	extra := attrsFromContext(ctx)
	if tid == "" && sid == "" && len(extra) == 0 {
		return h.inner.Handle(ctx, r)
	}
	r2 := r.Clone()
	if tid != "" {
		r2.AddAttrs(slog.String("trace_id", tid))
	}
	if sid != "" {
		r2.AddAttrs(slog.String("span_id", sid))
	}
	r2.AddAttrs(extra...)
	return h.inner.Handle(ctx, r2)
}

// WithAttrs 派生带固定字段的 handler。
// 输入 attrs：交给 inner.WithAttrs。
// 返回：新 *ctxHandler，原实例不变。
func (h *ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ctxHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup 派生带分组的 handler。
// 输入 name：交给 inner.WithGroup。
// 返回：新 *ctxHandler，原实例不变。
func (h *ctxHandler) WithGroup(name string) slog.Handler {
	return &ctxHandler{inner: h.inner.WithGroup(name)}
}
