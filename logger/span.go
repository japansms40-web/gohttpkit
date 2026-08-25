package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"
)

type spanIDKey struct{}

// SpanIDFromContext 从 ctx 提取当前 span_id;无则返回 ""。
// handler 层会把它自动附加到该 ctx 下每条日志,与 trace_id 同等待遇。
func SpanIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(spanIDKey{}).(string); ok {
		return v
	}
	return ""
}

// NewSpanID 生成 8 位 hex 短 span id(crypto/rand 4 字节);失败兜底时间戳。
// 比 trace_id(16 hex)短:span 数量多、仅需在单条 trace 内区分。
func NewSpanID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// DurationMs 统一的耗时字段(duration_ms,毫秒,int64),供非 span 场景(HTTP/WSS)直接使用,
// 保证全项目耗时字段同名同单位。
func DurationMs(d time.Duration) slog.Attr {
	return slog.Int64("duration_ms", d.Milliseconds())
}

// StartSpan 开启一个业务 span,构建轻量调用链(不引 OpenTelemetry SDK)。
//
// 行为:
//   - 确保 ctx 有 trace_id(EnsureTraceID),整条链共享。
//   - 生成新 span_id,把当前 ctx 已有的 span_id 记为 parent_span_id(嵌套关系)。
//   - 打 "span.start"(带 span_name / parent_span_id / 调用方入参 attrs)。
//   - 返回带新 span_id 的 ctx —— 此后该 ctx 下任意 logger.Info/Warn/Error 都自动带 span_id。
//   - 返回 end 回调:打 "span.end"(带 span_name / duration_ms / 调用方追加的结果 attrs)。
//
// 典型用法(命名返回值 + defer,自动记录退出与耗时):
//
//	func doStep(ctx context.Context) (out []byte, err error) {
//	    ctx, endSpan := logger.StartSpan(ctx, "register.step001_homepage")
//	    defer func() { endSpan(slog.Int("resp_len", len(out)), logger.Err(err)) }()
//	    ...
//	}
func StartSpan(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(end ...slog.Attr)) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = EnsureTraceID(ctx)
	parent := SpanIDFromContext(ctx)
	spanID := NewSpanID()
	ctx = context.WithValue(ctx, spanIDKey{}, spanID)

	start := time.Now()

	// span_id 由 handler 从 ctx 自动附加,这里不重复加;只补 span_name 与父链。
	startAttrs := make([]slog.Attr, 0, len(attrs)+2)
	startAttrs = append(startAttrs, slog.String("span_name", name))
	if parent != "" {
		startAttrs = append(startAttrs, slog.String("parent_span_id", parent))
	}
	startAttrs = append(startAttrs, attrs...)
	Info(ctx, "span.start", startAttrs...)

	end := func(endAttrs ...slog.Attr) {
		final := make([]slog.Attr, 0, len(endAttrs)+2)
		final = append(final, slog.String("span_name", name), DurationMs(time.Since(start)))
		final = append(final, endAttrs...)
		Info(ctx, "span.end", final...)
	}
	return ctx, end
}
