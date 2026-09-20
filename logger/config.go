package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// config.go —— 未注入外部 logger 时的默认输出构造。
// 独立成文件：Config / 落盘 / 级别解析和门面 Debug/Info 分开。

// Level 日志级别枚举。合法值见下列常量；其它值（空串、大小写、带空格）由 parseLevel 回落 LevelInfo。
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Format 默认 handler 的输出格式枚举。FormatConsole 用文本 handler，其它值一律按 JSON。
type Format string

const (
	FormatJSON    Format = "json"
	FormatConsole Format = "console"
)

// Output 默认 handler 的输出目标枚举。
type Output string

const (
	OutputConsole Output = "console"
	OutputFile    Output = "file"
	OutputBoth    Output = "both"
)

// Config 未注入外部 logger 时的默认输出行为。
// 非法 Level / Format 由 parseLevel / newDefault 回落，不报错（运维旋钮，打错字母不该让进程起不来）。
type Config struct {
	// Level 日志级别，取 Level* 常量之一。非法值回落 LevelInfo。
	Level Level
	// Format 输出格式，取 Format* 常量之一。非 FormatConsole 一律按 JSON。
	Format Format
	// Output 输出目标，取 Output* 常量之一。
	Output Output
	// FilePath 当输出为 OutputFile / OutputBoth 时使用；空则回落 ./logs/app.log。
	FilePath string
}

// DefaultConfig 返回未 SetConfig 时的默认配置。
// 输入：无。
// 返回：Level=info、Format=json、Output=console、FilePath=./logs/app.log。
// 例：DefaultConfig().Level == "info"。
// 默认值只在这里写一次，mustFileWriter 空路径从这里抄。
func DefaultConfig() Config {
	return Config{
		Level:    LevelInfo,
		Format:   FormatJSON,
		Output:   OutputConsole,
		FilePath: "./logs/app.log",
	}
}

// SetConfig 按 c 重建默认 logger。
// 输入 c：完整配置；零值字段按字面用（空 Level 会被 parseLevel 收成 info）。
// 返回：无。只改 fallback，不碰已注入的 external（SetHandler / SetLogger）。
// 例：SetConfig(Config{Level:"debug", Format:"json", Output:"console"}) 后未注入时走 debug。
func SetConfig(c Config) {
	fallback.Store(newDefault(c))
}

// newDefault 按 Config 构造已包装 ctx 注入的默认 logger。
// 输入 c：Output 认 file / both，其余当 console；Format=="console" 用文本，否则 JSON。
// 返回：非 nil *slog.Logger。落盘失败 panic *FileSetupError。
func newDefault(c Config) *slog.Logger {
	level := parseLevel(c.Level)

	var w io.Writer
	switch c.Output {
	case OutputFile:
		w = mustFileWriter(c.FilePath)
	case OutputBoth:
		w = io.MultiWriter(os.Stdout, mustFileWriter(c.FilePath))
	default: // console
		w = os.Stdout
	}

	opts := &slog.HandlerOptions{Level: level, AddSource: true}
	var h slog.Handler
	if c.Format == FormatConsole {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(wrapCtxHandler(h))
}

// mustFileWriter 打开日志文件，目录不存在则创建。
// 输入 path：目标文件；空串回落 DefaultConfig().FilePath。
// 返回：可追加写入的 *os.File（当作 io.Writer）。
// 失败 panic *FileSetupError（Op=mkdir 或 open），不返回 error——默认 logger
// 在启动期构造，半残 handler 比直接炸更难查。
func mustFileWriter(path string) io.Writer {
	if path == "" {
		path = DefaultConfig().FilePath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		panic(&FileSetupError{Path: path, Op: "mkdir", Err: err})
	}
	// #nosec G304 -- 日志路径本就来自接入方的配置/环境变量，是刻意的可配置项，不是外部输入
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(&FileSetupError{Path: path, Op: "open", Err: err})
	}
	return file
}

// parseLevel 把 Level 收成 slog.Level。
// 输入 level：只认 LevelDebug/LevelWarn/LevelError；LevelInfo 以及其它值（空串、大小写、空白）都回落 LevelInfo。
// 返回：对应级别。不做 trim——带空格视为非法，避免「看起来设了 warn 其实没设」。
// 例：parseLevel(LevelDebug) → slog.LevelDebug；parseLevel("DEBUG") → slog.LevelInfo。
func parseLevel(level Level) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
