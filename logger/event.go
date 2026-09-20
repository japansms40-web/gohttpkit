package logger

import "log/slog"

const (
	// EventSpanStart 是 StartSpan 开始节点的机器事件名，同时作为 msg。
	EventSpanStart = "span.start"
	// EventSpanEnd 是 StartSpan 结束节点的机器事件名，同时作为 msg。
	EventSpanEnd = "span.end"
)

// Event 返回机器可读的稳定事件字段。
// 输入 name：小写 ASCII 点分层事件名；空串也按字面保留。开放契约，接入方可自定
// account.sync.started 之类任意名，因此用字符串常量而不是 defined type——取值无法穷举，
// 且 JSON 反序列化后的 event 是 string，和 typed 常量比较会静默不相等。
// 封闭配置项（Level/Format/Output）才用 defined type。
// 返回：key 固定为 event 的 slog.Attr，不修改 Record.Message。
// 例：Event("http.retry") 输出 event=http.retry。
func Event(name string) slog.Attr {
	return slog.String("event", name)
}
