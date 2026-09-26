package logger

import (
	"context"
	"log/slog"
	"slices"
)

// named.go —— 不需要 ctx 的具名 Logger。
// 独立成文件：给拿不到 ctx 的调用点（main、init、启动配置、纯本地工具函数）用，
// 免得到处硬造 context.Background()；需要与 HTTP 日志同链时仍走包级 Info(ctx, ...)。
//
// Logger 只是进程级全局 logger 之上的一层字段视图：每次调用才读 active()，不缓存 handler，
// 所以包级 var 先于 SetHandler / SetConfig 创建也照样生效，多个模块共用同一份输出。

// Logger 不需要 ctx 的日志句柄，携带固定字段（module 与 With 追加的字段）。
// 零值与 nil 接收者都可用，等价于不带固定字段。
// 并发：实例不可变，With 返回新实例，可被多 goroutine 共享。
// 日志不带 trace_id / span_id（内部用 Background ctx）。
type Logger struct {
	attrs []slog.Attr
}

// Named 返回带 module 字段的 Logger，通常在包级声明一次。
// 输入 name：非空时每条日志带 module=name；空串不带 module 键。
// 返回：非 nil *Logger。
// 例：var log = logger.Named("insexecutor"); log.Info("任务开始")。
func Named(name string) *Logger {
	if name == "" {
		return &Logger{}
	}
	return &Logger{attrs: []slog.Attr{slog.String(FieldModule, name)}}
}

// With 派生追加固定字段的 Logger。
// 输入 attrs：追加在已有字段之后；为空时原样返回 l（包括 nil）。
// 返回：新 *Logger，copy-on-write，不改 l 与兄弟实例。
// 例：log.With(slog.String("account_id", "u1")).Warn("限流")。
func (l *Logger) With(attrs ...slog.Attr) *Logger {
	if len(attrs) == 0 {
		return l
	}
	return &Logger{attrs: slices.Concat(l.fixed(), attrs)}
}

// Debug 打 debug 级日志。
// 输入 msg / attrs：原样进 Record，排在固定字段之后。
// 返回：无。低于阈值时不写。
func (l *Logger) Debug(msg string, attrs ...slog.Attr) {
	output(context.Background(), slog.LevelDebug, msg, 3, l.fixed(), attrs)
}

// Info 打 info 级日志。
// 输入 msg / attrs：原样进 Record，排在固定字段之后。
// 返回：无。低于阈值时不写。
func (l *Logger) Info(msg string, attrs ...slog.Attr) {
	output(context.Background(), slog.LevelInfo, msg, 3, l.fixed(), attrs)
}

// Warn 打 warn 级日志。
// 输入 msg / attrs：原样进 Record，排在固定字段之后。
// 返回：无。低于阈值时不写。
func (l *Logger) Warn(msg string, attrs ...slog.Attr) {
	output(context.Background(), slog.LevelWarn, msg, 3, l.fixed(), attrs)
}

// Error 打 error 级日志。
// 输入 msg / attrs：原样进 Record，排在固定字段之后。
// 返回：无。低于阈值时不写。
func (l *Logger) Error(msg string, attrs ...slog.Attr) {
	output(context.Background(), slog.LevelError, msg, 3, l.fixed(), attrs)
}

// fixed 取固定字段。
// 输入：接收者可为 nil。
// 返回：nil 接收者返回 nil，否则返回只读切片，调用方不得改。
// skip=3 依赖四个级别方法直接调用 output（0=Callers 1=output 2=Debug/Info/Warn/Error 3=业务调用点），
// fixed 在进入 output 前已求值，不占调用栈帧。
func (l *Logger) fixed() []slog.Attr {
	if l == nil {
		return nil
	}
	return l.attrs
}
