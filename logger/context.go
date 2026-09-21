package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

// context.go —— trace_id 与 ctx 通用字段注入。
// 独立成文件：ctx key 与 handler 包装、span 生命周期分开，避免把注入散落到门面文件。

type traceIDKey struct{}

type ctxAttrsKey struct{}

// WithTraceID 把 trace_id 写入 ctx，此后该 ctx 下所有日志自动携带。
// 输入 ctx：nil 回落 Background；traceID：原样存，空串也写入（EnsureTraceID 会当成没有再生成）。
// 返回：派生 ctx，不改入参。
// 例：WithTraceID(ctx, "abc123")。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext 从 ctx 取出 trace_id。
// 输入 ctx：nil 或未注入返回 ""。
// 返回：注入过的字符串，可能是空串。
// 例：TraceIDFromContext(WithTraceID(ctx, "abc")) → "abc"。
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

// EnsureTraceID 保证 ctx 里有非空 trace_id。
// 输入 ctx：nil 回落 Background；已有非空 trace_id 则原样保留（不改格式、不截断）。
// 返回：带 trace_id 的派生 ctx，必须写成 ctx = EnsureTraceID(ctx)。
// 只有自动生成的值才是 16 位 hex；调用方显式注入的非空值始终优先。
// 例：EnsureTraceID(Background) 后是 16 位 hex；EnsureTraceID(WithTraceID(ctx, "abc")) 仍是 abc。
func EnsureTraceID(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if TraceIDFromContext(ctx) != "" {
		return ctx
	}
	return WithTraceID(ctx, NewTraceID())
}

// NewTraceID 生成 16 位 hex 短追踪 id。
// 输入：无。
// 返回：crypto/rand 8 字节的 hex；读随机数失败则用 UnixNano 按 16 位兜底。
func NewTraceID() string { return newRandomHex(8, 16) }

// readRandom 默认 crypto/rand.Read；测试可替换以覆盖失败兜底。
var readRandom = rand.Read

// newRandomHex 生成 nBytes 的 hex。
// 输入 nBytes：随机字节数；hexWidth：失败兜底时的 hex 宽度。
// 返回：2*nBytes 长的 hex；rand.Read 失败时用 UnixNano 按 hexWidth 位补零。
// NewTraceID / NewSpanID 必须共用，否则两边的失败兜底格式会漂。
func newRandomHex(nBytes, hexWidth int) string {
	b := make([]byte, nBytes)
	if _, err := readRandom(b); err != nil {
		return fmt.Sprintf("%0*x", hexWidth, time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// WithAttrs 把通用日志字段注入 ctx（如 account_id、task_id）。
// 输入 ctx：nil 回落 Background；attrs：空则原样返回 ctx（不分配）。
// 返回：派生 ctx。多次调用追加；copy-on-write，不改父 ctx 已存切片。
// 例：WithAttrs(ctx, slog.String("account_id", "u1"))。
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	old := attrsFromContext(ctx)
	merged := make([]slog.Attr, 0, len(old)+len(attrs))
	merged = append(append(merged, old...), attrs...)
	return context.WithValue(ctx, ctxAttrsKey{}, merged)
}

// attrsFromContext 取出 WithAttrs 注入的通用字段。
// 输入 ctx：nil 或未注入返回 nil。
// 返回：只读切片，调用方不得改。
func attrsFromContext(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(ctxAttrsKey{}).([]slog.Attr)
	return v
}
