package logger

import (
	"context"
	"log/slog"
	"time"
)

// span.go —— 轻量 span（不引 OpenTelemetry）。独立成文件：生命周期与门面、ctx 注入分开。

type spanIDKey struct{}

// SpanIDFromContext 从 ctx 取出当前 span_id。
// 输入 ctx：nil 或未开启 span 返回 ""。
// 返回：StartSpan 写入的短 id。
// 例：SpanIDFromContext(ctx) 在 span 内非空，外面是 ""。
// handler 层会把它自动附加到该 ctx 下每条日志，与 trace_id 同等待遇。
func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(spanIDKey{}).(string); ok {
		return v
	}
	return ""
}

// NewSpanID 生成 8 位 hex 短 span id。
// 输入：无。
// 返回：crypto/rand 4 字节的 hex；失败兜底时间戳。
// 比 trace_id（16 hex）短：span 数量多、仅需在单条 trace 内区分。
func NewSpanID() string { return newRandomHex(4, 8) }

// DurationMs 统一的耗时字段。
// 输入 d：任意 duration，负值按 slog 原样变成负毫秒。
// 返回：键名固定 duration_ms，单位毫秒，int64。
// 例：DurationMs(1500*time.Millisecond) 的值为 1500。
// 供非 span 场景（HTTP/WSS）直接使用，保证全项目耗时字段同名同单位。
func DurationMs(d time.Duration) slog.Attr {
	return slog.Int64(FieldDurationMS, d.Milliseconds())
}

// StartSpan 开启一个业务 span。
// 输入 ctx：nil 回落 Background；name：span_name；attrs：打进 span.start。
// 返回：带新 span_id 且已 EnsureTraceID 的派生 ctx，以及 end 回调。
// 必须把返回 ctx 往下传（含 httpx.Do / 内层拦截器）；丢掉它等于没开 span。
// end 用该 span ctx 打 span.end（span_name / duration_ms / 追加字段），可多次调用，每次再打一条 end。
// 例：ctx, end := StartSpan(ctx, "register.step"); defer end()。
func StartSpan(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(end ...slog.Attr)) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = EnsureTraceID(ctx)
	parent := SpanIDFromContext(ctx)
	spanID := NewSpanID()
	ctx = context.WithValue(ctx, spanIDKey{}, spanID)

	start := time.Now()

	// span_id 由 handler 从 ctx 自动附加，这里不重复加；只补 span_name 与父链。
	startAttrs := make([]slog.Attr, 0, len(attrs)+2)
	startAttrs = append(startAttrs, slog.String(FieldSpanName, name))
	if parent != "" {
		startAttrs = append(startAttrs, slog.String(FieldParentSpanID, parent))
	}
	startAttrs = append(startAttrs, attrs...)
	InfoEvent(ctx, EventSpanStart, startAttrs...)

	end := func(endAttrs ...slog.Attr) {
		final := make([]slog.Attr, 0, len(endAttrs)+2)
		final = append(final, slog.String(FieldSpanName, name), DurationMs(time.Since(start)))
		final = append(final, endAttrs...)
		InfoEvent(ctx, EventSpanEnd, final...)
	}
	return ctx, end
}
