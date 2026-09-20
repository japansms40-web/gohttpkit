package logger

import (
	"context"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"
)

// logger.go —— 门面 API 与当前生效 logger 的选取。
// 独立成文件：接入方只需要认这几个函数，env / handler / span 的实现细节不该和 Debug/Info 挤在一起。
//
// 并发：external 与 fallback 均为 atomic.Pointer，SetLogger / SetHandler / SetConfig / active
// 可被多 goroutine 同时调用。没有互斥锁；替换的是整份 *slog.Logger 引用。

var (
	external atomic.Pointer[slog.Logger] // SetLogger/SetHandler 注入；nil = 未注入
	fallback atomic.Pointer[slog.Logger] // 默认 logger，SetConfig 重建，首次使用时惰性初始化
)

// SetLogger 注入外部 *slog.Logger，优先于默认 logger。
// 输入 l：非 nil 时取其 Handler 再包一层 ctx 注入；nil 清空 external，回到 fallback。
// 返回：无。
// 例：SetLogger(slog.New(h)) 后 Info 走 h；SetLogger(nil) 还原默认。
// 注入的 handler 会被包装，自动获得 ctx 中的 trace_id 与 WithAttrs 字段。
func SetLogger(l *slog.Logger) {
	if l == nil {
		external.Store(nil)
		return
	}
	external.Store(slog.New(wrapCtxHandler(l.Handler())))
}

// SetHandler 注入外部 slog.Handler（接入自家日志的主入口）。
// 输入 h：zap 可传 zapslog.NewHandler(core)；nil 清空 external，回到 fallback。
// 返回：无。
// 例：SetHandler(slog.NewJSONHandler(w, nil))。
func SetHandler(h slog.Handler) {
	if h == nil {
		external.Store(nil)
		return
	}
	external.Store(slog.New(wrapCtxHandler(h)))
}

// Default 返回当前生效的 logger（已含 ctx 字段自动注入）。
// 输入：无。
// 返回：非 nil *slog.Logger，可转交给其它库。
// 例：Default().Info("via-default")。
func Default() *slog.Logger { return active() }

// active 选取当前 logger。
// 输入：无（读进程级 external / fallback）。
// 返回：external 优先；否则 fallback；都空则按 DefaultConfig 惰性创建并 CAS 回填。
func active() *slog.Logger {
	if l := external.Load(); l != nil {
		return l
	}
	if l := fallback.Load(); l != nil {
		return l
	}
	l := newDefault(DefaultConfig())
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

// log 构造 Record 并转发给当前 handler。
// 输入 ctx：nil 回落 Background；level / msg / attrs 写入 Record。
// 返回：无。Handler.Handle 的 error 被丢掉——日志失败不能反向打爆业务。
// runtime.Callers(3)：0=Callers 1=log 2=Debug/Info/Warn/Error 3=业务调用点，保证 AddSource 定位正确。
func log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if ctx == nil {
		ctx = context.Background()
	}
	l := active()
	if !l.Handler().Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.AddAttrs(attrs...)
	_ = l.Handler().Handle(ctx, r)
}

// Err 错误字段（替代 zap.Error）。
// 输入 err：nil 返回零值 Attr，handler 按 slog 规范忽略。
// 返回：键名固定 "error"，值是 err.Error()。
// 例：Err(io.EOF) → slog.String("error", "EOF")；Err(nil) → slog.Attr{}。
func Err(err error) slog.Attr {
	return NamedErr("error", err)
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
