package logger

import (
	"context"
	"log/slog"
	"slices"
)

var (
	// EventSpanStart 是 StartSpan 开始节点的机器事件。
	// 给 StartSpan 与接入方按 event=span.start 过滤；msg 与 Name 同值。
	EventSpanStart = NewEvent("span.start")
	// EventSpanEnd 是 StartSpan 结束节点的机器事件。
	// 给 StartSpan 的 end 回调与接入方按 event=span.end 过滤；msg 与 Name 同值。
	EventSpanEnd = NewEvent("span.end")
)

// Event 是可跨包实现的机器事件；Name 应返回稳定名称。
// 给 httpx 与接入方自定 account.sync.started 之类事件。不校验、不归一化。
type Event interface {
	Name() string
}

type namedEvent string

// Name 返回默认 event 的原始名称。
// 输入：值接收者 e。
// 返回：创建时的名称，包括空串。
func (e namedEvent) Name() string { return string(e) }

// NewEvent 用 name 构造不可变的默认 Event；空串按字面保留。
// 输入：name 为机器事件名，不校验、不归一化。
// 返回：动态值为 namedEvent 的 Event。
// 例：NewEvent("http.retry").Name() == "http.retry"；NewEvent("").Name() == ""。
func NewEvent(name string) Event { return namedEvent(name) }

// EventAttr 把字符串事件名转为 key 固定为 event 的 slog.Attr。
// 输入：name 按字面使用，可为空。
// 返回：slog.String(FieldEvent, name)。
// 例：Info(ctx, "给人看的文案", EventAttr("demo.finished")) 的 msg 仍是人类文案。
func EventAttr(name string) slog.Attr { return slog.String(FieldEvent, name) }

// Attr 把 Event 转为 slog.Attr；nil 接口输出 event=""。
// 输入：e 可为 nil 接口；typed-nil 由其 Name 实现负责。
// 返回：key 固定为 event 的 slog.Attr。
func Attr(e Event) slog.Attr { return EventAttr(eventName(e)) }

// eventName 安全取名；nil 接口返回空串，typed-nil 交给其 Name 实现。
// 输入：e 可为 nil 接口。
// 返回：nil 时为空串，否则为一次 e.Name() 的结果。
func eventName(e Event) string {
	if e == nil {
		return ""
	}
	return e.Name()
}

// attrsWithEvent 克隆 attrs 后追加 event，不覆盖调用方容量区域。
// 输入：attrs 可为 nil；name 是已求值一次的事件名。
// 返回：独立底层数组的 attrs 副本，event 在最后一位。
func attrsWithEvent(attrs []slog.Attr, name string) []slog.Attr {
	cloned := slices.Clone(attrs)
	return append(cloned, EventAttr(name))
}

// InfoEvent 打 info 事件日志；Name 只求值一次，msg 与 event 同值。
// 输入：ctx 可为 nil；e 可为 nil 接口；attrs 不会被修改。
// 返回：无；级别过滤和 handler 错误处理沿用 log。
// 例：InfoEvent(ctx, NewEvent("account.sync.started")) → msg=event=account.sync.started。
func InfoEvent(ctx context.Context, e Event, attrs ...slog.Attr) {
	name := eventName(e)
	log(ctx, slog.LevelInfo, name, attrsWithEvent(attrs, name)...)
}

// WarnEvent 打 warn 事件日志；Name 只求值一次，msg 与 event 同值。
// 输入：ctx 可为 nil；e 可为 nil 接口；attrs 不会被修改。
// 返回：无；级别过滤和 handler 错误处理沿用 log。
func WarnEvent(ctx context.Context, e Event, attrs ...slog.Attr) {
	name := eventName(e)
	log(ctx, slog.LevelWarn, name, attrsWithEvent(attrs, name)...)
}
