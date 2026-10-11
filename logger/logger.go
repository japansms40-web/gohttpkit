package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

// logger.go —— 门面 API 与当前生效 logger 的选取。
// 独立成文件：接入方只需要认这几个函数，配置 / handler / span 的实现细节不该和 Debug/Info 挤在一起。
//
// 并发：installed 与 fallback 均为 atomic.Pointer，install / active 可被多 goroutine 同时调用。
// 打日志不拿锁；替换的是整份 *slog.Logger 引用。

var (
	installed atomic.Pointer[slog.Logger] // Init 安装的 logger；nil = 没有初始化，走 fallback
	fallback  atomic.Pointer[slog.Logger] // 缺省 logger，没有初始化时使用，首次使用时惰性创建
)

// install 把 h 装成当前生效的输出，是 Init 之下唯一的安装点。
// 输入 h：非 nil 时包一层 ctx 注入后生效，自动获得 ctx 中的 trace_id、span_id 与 WithAttrs 字段；nil 清空，回到缺省输出。
// 返回：无。
func install(h slog.Handler) {
	if h == nil {
		installed.Store(nil)
		return
	}
	installed.Store(slog.New(wrapCtxHandler(h)))
}

// Enabled 报告当前输出在 level 级别是否会写出。
// 输入 ctx：可为 nil（回落 Background）；level：slog 级别。
// 返回：会写出为 true。供调用方在拼装昂贵字段（如完整 SQL 文本）之前判断，省掉注定被丢弃的开销。
// 例：if logger.Enabled(ctx, slog.LevelDebug) { logger.DebugEvent(ctx, ev, slog.String("sql", explain())) }。
func Enabled(ctx context.Context, level slog.Level) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	return active().Handler().Enabled(ctx, level)
}

// active 选取当前 logger。
// 输入：无（读进程级 installed / fallback）。
// 返回：Init 安装的优先；否则 fallback；都空则按 DefaultConfig 惰性创建并 CAS 回填。
func active() *slog.Logger {
	if l := installed.Load(); l != nil {
		return l
	}
	if l := fallback.Load(); l != nil {
		return l
	}
	l := slog.New(wrapCtxHandler(buildHandler(os.Stdout, DefaultConfig(), nil)))
	if fallback.CompareAndSwap(nil, l) {
		return l
	}
	return fallback.Load()
}

// Debug 打 debug 级日志。
// 输入 ctx：可为 nil（回落 Background）；msg / attrs 原样进 Record。
// 返回：无。低于阈值时不写。trace_id 与 ctx 通用字段由 handler 附加。
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

// output 构造 Record 并转发给当前 handler。
// 输入 ctx：非 nil；skip：runtime.Callers 的跳过帧数，须指向业务调用点；
// pre：先写入的固定字段（Logger 的 module / With）；attrs：本次调用字段，排在 pre 之后。
// 返回：无。Handler.Handle 的 error 被丢掉——日志失败不能反向打爆业务。
// 先判 Enabled 再取调用栈：低于阈值时不付 runtime.Callers 的开销。
func output(ctx context.Context, level slog.Level, msg string, skip int, pre, attrs []slog.Attr) {
	h := active().Handler()
	if !h.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(skip, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.AddAttrs(pre...)
	r.AddAttrs(attrs...)
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
