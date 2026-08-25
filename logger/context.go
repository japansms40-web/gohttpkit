package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

type traceIDKey struct{}

type ctxAttrsKey struct{}

// WithTraceID 将 trace_id 注入到 context 中,此后该 ctx 下所有日志自动携带
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext 从 context 中提取 trace_id
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

// EnsureTraceID 确保 ctx 中有 trace_id:已有则原样返回,没有则生成短 id 注入。
// 调用方显式注入的 trace_id 始终优先,SDK 仅在入口处兜底。
func EnsureTraceID(ctx context.Context) context.Context {
	if TraceIDFromContext(ctx) != "" {
		return ctx
	}
	return WithTraceID(ctx, NewTraceID())
}

// NewTraceID 生成 16 位 hex 短追踪 id(crypto/rand 8 字节)
func NewTraceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// WithAttrs 把通用日志字段注入 ctx(如 account_id、task_id),此后该 ctx 下所有日志自动附带。
// 多次调用为追加;copy-on-write,不修改父 ctx 已存的字段切片。
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	old := attrsFromContext(ctx)
	merged := make([]slog.Attr, 0, len(old)+len(attrs))
	merged = append(append(merged, old...), attrs...)
	return context.WithValue(ctx, ctxAttrsKey{}, merged)
}

// attrsFromContext 提取 WithAttrs 注入的通用字段;返回的切片只读,不得修改
func attrsFromContext(ctx context.Context) []slog.Attr {
	v, _ := ctx.Value(ctxAttrsKey{}).([]slog.Attr)
	return v
}
