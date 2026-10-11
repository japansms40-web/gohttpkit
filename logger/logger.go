package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

// logger.go —— 门面 API、当前生效的 logger，以及全部门面日志的唯一出口 output。
// 独立成文件：接入方只需要认这几个函数，配置 / span 的实现细节不该和 Debug/Info 挤在一起。
//
// 并发：current 是 atomic.Pointer，Init / Close 替换的是整份 *slog.Logger 引用，打日志不拿锁。

// current 当前生效的 logger，任何时刻都非 nil：包加载时放好缺省输出，Init 换掉，Close 换回。
var current atomic.Pointer[slog.Logger]

// init 在包加载时放好缺省输出，没有调用 Init 的进程也能打日志。
// 输入：无。
// 返回：无。缺省输出按 DefaultConfig：控制台 JSON、info 级。
func init() {
	current.Store(slog.New(buildHandler(os.Stdout, DefaultConfig(), nil)))
}

// Enabled 报告当前输出在 level 级别是否会写出。
// 输入 ctx：可为 nil（回落 Background）；level：slog 级别。
// 返回：会写出为 true。供调用方在拼装昂贵字段（如完整 SQL 文本）之前判断，省掉注定被丢弃的开销。
// 例：if logger.Enabled(ctx, slog.LevelDebug) { logger.DebugEvent(ctx, ev, slog.String("sql", explain())) }。
func Enabled(ctx context.Context, level slog.Level) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	return current.Load().Handler().Enabled(ctx, level)
}

// Debug 打 debug 级日志。
// 输入 ctx：可为 nil（回落 Background）；msg / attrs 原样进 Record。
// 返回：无。低于阈值时不写。trace_id、span_id 与 ctx 通用字段由 output 附加。
func Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelDebug, msg, attrs...)
}

// Info 打 info 级日志。
// 输入 ctx：可为 nil（回落 Background）；msg / attrs 原样进 Record。
// 返回：无。低于阈值时不写。
func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn 打 warn 级日志。
// 输入 ctx：可为 nil（回落 Background）；msg / attrs 原样进 Record。
// 返回：无。低于阈值时不写。
func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelWarn, msg, attrs...)
}

// Error 打 error 级日志。
// 输入 ctx：可为 nil（回落 Background）；msg / attrs 原样进 Record。
// 返回：无。低于阈值时不写。
func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelError, msg, attrs...)
}

// log 是包级 ctx 门面（Debug/Info/Warn/Error、DebugEvent/InfoEvent/WarnEvent/ErrorEvent）的公共出口。
// 输入 ctx：nil 回落 Background；level / msg / attrs 写入 Record。
// 返回：无。skip=4：0=Callers 1=output 2=log 3=门面函数 4=业务调用点，保证 AddSource 定位正确。
func log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if ctx == nil {
		ctx = context.Background()
	}
	output(ctx, level, msg, 4, nil, attrs)
}

// output 构造 Record 并交给当前输出，是全部门面日志的唯一出口。
// 输入 ctx：非 nil；skip：runtime.Callers 的跳过帧数，须指向业务调用点；
// pre：先写入的固定字段（Logger 的 module / With）；attrs：本次调用字段，排在 pre 之后。
// 返回：无。Handler.Handle 的 error 被丢掉——日志失败不能反向打爆业务。
// 字段顺序：pre、attrs，然后是 ctx 里的 trace_id、span_id 与 WithAttrs 通用字段；ctx 里没有的不写。
// 先判 Enabled 再取调用栈：低于阈值时不付 runtime.Callers 的开销。
func output(ctx context.Context, level slog.Level, msg string, skip int, pre, attrs []slog.Attr) {
	h := current.Load().Handler()
	if !h.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(skip, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.AddAttrs(pre...)
	r.AddAttrs(attrs...)
	if tid := TraceIDFromContext(ctx); tid != "" {
		r.AddAttrs(slog.String(FieldTraceID, tid))
	}
	if sid := SpanIDFromContext(ctx); sid != "" {
		r.AddAttrs(slog.String(FieldSpanID, sid))
	}
	r.AddAttrs(attrsFromContext(ctx)...)
	_ = h.Handle(ctx, r)
}

// Err 错误字段（替代 zap.Error）。
// 输入 err：nil 返回零值 Attr，handler 按 slog 规范忽略。
// 返回：键名固定 "error"，值是 err.Error()。
// 例：Err(io.EOF) → slog.String("error", "EOF")；Err(nil) → slog.Attr{}。
func Err(err error) slog.Attr {
	return NamedErr(FieldError, err)
}

// NamedErr 自定义键名的错误字段（替代 zap.NamedError）。
// 输入 key：字段名；err：nil 返回零值 Attr。
// 返回：slog.String(key, err.Error()) 或零值。
// 例：NamedErr("classify_err", context.Canceled)。
func NamedErr(key string, err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String(key, err.Error())
}
