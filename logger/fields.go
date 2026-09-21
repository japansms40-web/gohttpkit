package logger

// fields.go —— 结构化日志字段 key。单一事实源，禁止在 handler / span / EventAttr 处再写同名字面量。
// 不作 type 枚举：它们只当 slog.Attr 的 key，套 type 只会引入转换噪音。

const (
	FieldEvent        = "event"
	FieldTraceID      = "trace_id"
	FieldSpanID       = "span_id"
	FieldSpanName     = "span_name"
	FieldError        = "error"
	FieldParentSpanID = "parent_span_id"
	FieldDurationMS   = "duration_ms"
)
