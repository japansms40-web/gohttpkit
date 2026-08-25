// Package logger 提供基于 log/slog 的统一日志门面。
//
// 设计要点:
//   - 统一 API:Debug/Info/Warn/Error(ctx, msg, attrs...),ctx 必传首参;
//     trace_id 与 WithAttrs 注入的通用字段在 handler 层自动附加,业务代码零传参。
//   - 注入:调用方通过 SetHandler/SetLogger 注入自家日志实现
//     (zap 用 zapslog.NewHandler,logrus/zerolog 等同理),注入的 handler 同样自动获得 trace_id。
//   - 未注入时按 Config 输出(默认 console JSON / info 级)。
//   - 环境变量自动配置(env.go):设 HTTPKIT_LOG_FILE=./logs/app.log 即把稳定 JSON 日志
//     落盘到该文件,供运维 / AI 离线 grep;按 trace_id 串链、按 span_name 过滤。
//     另有 HTTPKIT_LOG_LEVEL / HTTPKIT_LOG_FORMAT / HTTPKIT_LOG_OUTPUT。不设则行为不变。
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
)

// Config 日志配置(未注入外部 logger 时的默认输出行为)
type Config struct {
	Level    string // debug, info, warn, error
	Format   string // json 或 console
	Output   string // console, file, both
	FilePath string // 当输出包含 file 时使用
}

var (
	external atomic.Pointer[slog.Logger] // SetLogger/SetHandler 注入;nil = 未注入
	fallback atomic.Pointer[slog.Logger] // 默认 logger,SetConfig 重建,首次使用时惰性初始化
)

// SetConfig 配置默认 logger(未注入外部 logger 时生效),任何时刻调用都会重建默认实例
func SetConfig(c Config) {
	fallback.Store(newDefault(c))
}

// SetLogger 注入外部 logger,优先于默认 logger;传 nil 还原默认。
// 注入的 handler 会被包装,自动获得 ctx 中的 trace_id 与 WithAttrs 字段。
func SetLogger(l *slog.Logger) {
	if l == nil {
		external.Store(nil)
		return
	}
	external.Store(slog.New(wrapCtxHandler(l.Handler())))
}

// SetHandler 注入外部 slog.Handler(SDK 调用方接入自家日志的主入口);传 nil 还原默认。
// zap 调用方可传 zapslog.NewHandler(core),logrus/zerolog 等用各自的 slog 桥。
func SetHandler(h slog.Handler) {
	if h == nil {
		external.Store(nil)
		return
	}
	external.Store(slog.New(wrapCtxHandler(h)))
}

// Default 返回当前生效的 logger(已含 ctx 字段自动注入),可转交给其它库使用
func Default() *slog.Logger { return active() }

func active() *slog.Logger {
	if l := external.Load(); l != nil {
		return l
	}
	if l := fallback.Load(); l != nil {
		return l
	}
	l := newDefault(Config{Level: "info", Format: "json", Output: "console", FilePath: "./logs/app.log"})
	if fallback.CompareAndSwap(nil, l) {
		return l
	}
	return fallback.Load()
}

// Debug 打 debug 级日志,trace_id 与 ctx 通用字段自动附加
func Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelDebug, msg, attrs...)
}

// Info 打 info 级日志,trace_id 与 ctx 通用字段自动附加
func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn 打 warn 级日志,trace_id 与 ctx 通用字段自动附加
func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelWarn, msg, attrs...)
}

// Error 打 error 级日志,trace_id 与 ctx 通用字段自动附加
func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	log(ctx, slog.LevelError, msg, attrs...)
}

// log 构造 Record 并转发给当前 handler。
// runtime.Callers(3):0=Callers 1=log 2=Debug/Info/Warn/Error 3=业务调用点,保证 AddSource 定位正确
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

// Err 错误字段(替代 zap.Error);err 为 nil 时返回零值 Attr,handler 按 slog 规范忽略
func Err(err error) slog.Attr {
	return NamedErr("error", err)
}

// NamedErr 自定义键名的错误字段(替代 zap.NamedError);nil 安全
func NamedErr(key string, err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String(key, err.Error())
}

// newDefault 按 Config 构造默认 logger
func newDefault(c Config) *slog.Logger {
	level := parseLevel(c.Level)

	var w io.Writer
	switch c.Output {
	case "file":
		w = mustFileWriter(c.FilePath)
	case "both":
		w = io.MultiWriter(os.Stdout, mustFileWriter(c.FilePath))
	default: // console
		w = os.Stdout
	}

	opts := &slog.HandlerOptions{Level: level, AddSource: true}
	var h slog.Handler
	if c.Format == "console" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(wrapCtxHandler(h))
}

// mustFileWriter 打开日志文件(目录自动创建),失败 panic(与原实现语义一致)
func mustFileWriter(path string) io.Writer {
	if path == "" {
		path = "./logs/app.log"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		panic(fmt.Sprintf("failed to create log directory: %v", err))
	}
	// #nosec G304 -- 日志路径本就来自接入方的配置/环境变量，是刻意的可配置项，不是外部输入
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(fmt.Sprintf("failed to open log file: %v", err))
	}
	return file
}

// parseLevel 解析日志级别
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
