package logger

import (
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"sync"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"
)

// config.go —— 进程日志的配置与唯一初始化入口：Config、Init、Close，以及输出、轮转与处理器的构造。
// 独立成文件：配置 / 落盘 / 级别解析和门面 Debug/Info 分开。

// Level 日志级别枚举。合法值见下列常量；其它值（空串、大小写、带空格）由 parseLevel 回落 LevelInfo。
type Level string

// Level 的合法取值，由低到高。parseLevel 只认这四个，其它一律回落 LevelInfo。
const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Format 输出格式枚举。FormatConsole 用文本 handler，其它值一律按 JSON。
type Format string

// Format 的合法取值。仅 FormatConsole 走文本 handler，其余一律按 JSON。
const (
	FormatJSON    Format = "json"
	FormatConsole Format = "console"
)

// Output 输出目标枚举。
type Output string

// Output 的合法取值。OutputBoth 同时写控制台与文件。
const (
	OutputConsole Output = "console"
	OutputFile    Output = "file"
	OutputBoth    Output = "both"
)

// Config 进程日志的全部配置，交给 Init。
// 非法 Level / Format / Output 按缺省处理，不报错（运维旋钮，打错字母不该让进程起不来）。
type Config struct {
	// Level 日志级别，取 Level* 常量之一。非法值回落 LevelInfo。
	Level Level
	// Format 输出格式，取 Format* 常量之一。非 FormatConsole 一律按 JSON。
	Format Format
	// Output 输出目标，取 Output* 常量之一。非 OutputFile / OutputBoth 一律按控制台。
	Output Output
	// Filename 日志文件路径，Output 为 OutputFile / OutputBoth 时使用；空则回落 ./logs/app.log。
	Filename string
	// MaxSize 单个日志文件的大小上限，单位 MB；不大于 0 时用轮转库的缺省值 100。
	MaxSize int
	// MaxBackups 保留的历史文件个数；不大于 0 时不按个数清理。
	MaxBackups int
	// MaxAge 历史文件的保留天数；不大于 0 时不按天数清理。
	MaxAge int
	// Compress 是否用 gzip 压缩历史文件。
	Compress bool
	// Writer 控制台这一路的去向；nil 用 os.Stdout。不属于配置文件项，给测试和特殊接入用。
	Writer io.Writer
}

// DefaultConfig 返回没有调用 Init 时使用的缺省配置。
// 输入：无。
// 返回：Level=info、Format=json、Output=console、Filename=./logs/app.log，轮转四项为零值。
// 例：DefaultConfig().Level == "info"。
// 缺省值只在这里写一次，openLogFile 的空路径从这里抄。
func DefaultConfig() Config {
	return Config{
		Level:    LevelInfo,
		Format:   FormatJSON,
		Output:   OutputConsole,
		Filename: "./logs/app.log",
	}
}

// session 是一次 Init 留下的可撤销状态：Close 按它把进程日志还原到那次 Init 之前。
// 并发：只在持有 lifecycleMu 时读写。
type session struct {
	// parent 上一次还没撤销的 Init；nil 表示本次是最外层。
	parent *session
	// file 本次 Init 打开的日志文件；没有落盘时为 nil。
	file *lumberjack.Logger
	// prevInstalled 本次 Init 之前生效的 logger；nil 表示之前走缺省输出。
	prevInstalled *slog.Logger
	// prevDefault 本次 Init 之前的 slog 默认 logger。
	prevDefault *slog.Logger
	// prevOut 本次 Init 之前标准库 log 的输出。
	prevOut io.Writer
	// prevFlags 本次 Init 之前标准库 log 的标志。
	prevFlags int
}

var (
	// lifecycleMu 串行化 Init 与 Close，保护 live。打日志不拿这把锁。
	lifecycleMu sync.Mutex
	// live 最近一次还没撤销的 Init；nil 表示没有。
	live *session
)

// Init 按 c 接管进程日志，是唯一的初始化入口，进程启动时调用一次。
// 一次做完：按 Output 建输出（落盘走轮转）、按 Format 与 Level 建处理器、把 attrs 挂成每行都带的固定字段、
// 装成当前生效的输出，并把同一个处理器设成 slog 的默认处理器——标准库 log 的输出随之并入同一条通道。
// 输入 c：见 Config，零值等于控制台 JSON、info 级。attrs：固定字段（服务名、环境等），可为空，不得与保留字段重名。
// 返回：成功为 nil。日志文件建不出来或打不开时返回 *FileSetupError（Op=open），此时进程日志状态完全不变。
// 例：Init(Config{Level: LevelInfo, Output: OutputBoth, Filename: "./logs/app.log", MaxSize: 200}, slog.String("app", "svc"))。
// 可以嵌套：每次 Init 都记下之前的状态，由对应的那次 Close 还原，后进先出。
func Init(c Config, attrs ...slog.Attr) error {
	w, file, err := openOutput(c)
	if err != nil {
		return err
	}
	h := buildHandler(w, c, attrs)

	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	live = &session{
		parent:        live,
		file:          file,
		prevInstalled: installed.Load(),
		prevDefault:   slog.Default(),
		prevOut:       stdlog.Writer(),
		prevFlags:     stdlog.Flags(),
	}
	install(h)
	// net/http 等标准库在出错路径直接用 log.Printf，第三方库也可能直接用 slog 默认 logger；
	// 把默认 logger 换成同一个处理器，这些输出才不会绕过本通道、破坏结构化日志采集。
	slog.SetDefault(installed.Load())
	return nil
}

// Close 撤销最近一次 Init：关闭那次打开的日志文件，并把当前输出、slog 默认 logger、
// 标准库 log 的输出与标志都还原到那次 Init 之前。
// 输入：无。
// 返回：无。没有可撤销的 Init 时什么都不做。关闭日志文件的错误被丢掉——收尾阶段的日志故障不该反向打爆业务，也没有人能处理它。
// 例：Init(a); Init(b); Close() → 回到 a；再 Close() → 回到缺省输出；再 Close() → 无事发生。
func Close() {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	s := live
	if s == nil {
		return
	}
	live = s.parent
	installed.Store(s.prevInstalled)
	// 上一个默认 logger 若是标准库原生的那个，SetDefault 不会动标准库 log 的输出，所以输出与标志要显式写回。
	slog.SetDefault(s.prevDefault)
	stdlog.SetOutput(s.prevOut)
	stdlog.SetFlags(s.prevFlags)
	if s.file != nil {
		_ = s.file.Close()
	}
}

// openOutput 按 c.Output 建输出。
// 输入 c：Output 认 file / both，其余当控制台；控制台一路取 c.Writer，nil 用 os.Stdout。
// 返回：写入目标；本次打开的日志文件（没有落盘时为 nil）；落盘失败时前两项为 nil，error 是 *FileSetupError。
func openOutput(c Config) (io.Writer, *lumberjack.Logger, error) {
	console := c.Writer
	if console == nil {
		console = os.Stdout
	}
	if c.Output != OutputFile && c.Output != OutputBoth {
		return console, nil, nil
	}
	file, err := openLogFile(c)
	if err != nil {
		return nil, nil, err
	}
	if c.Output == OutputBoth {
		return io.MultiWriter(console, file), file, nil
	}
	return file, file, nil
}

// openLogFile 构造带轮转的日志文件，并立即做一次零长度写入，让轮转库当场建目录、打开文件。
// 输入 c：Filename 为空回落 DefaultConfig().Filename；MaxSize / MaxBackups / MaxAge 为负按 0（交给轮转库缺省）。
// 返回：已打开的日志文件。路径是目录、目录建不出来或文件打不开时返回 nil 和 *FileSetupError（Op=open），不留半开的句柄。
// 已有文件以追加方式打开，不覆盖；新文件权限 0600、新目录权限 0755，由轮转库决定。
// 启动时就探测，是为了把「日志落不了盘」变成启动失败，而不是运行中静默丢日志。
// 路径是目录要自己先挡：轮转库遇到打不开的同名路径会把它改名成备份再新建文件，等于把调用方的目录挪走。
func openLogFile(c Config) (*lumberjack.Logger, error) {
	path := c.Filename
	if path == "" {
		path = DefaultConfig().Filename
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, &FileSetupError{Path: path, Op: "open", Err: &os.PathError{Op: "open", Path: path, Err: syscall.EISDIR}}
	}
	file := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    max(c.MaxSize, 0),
		MaxBackups: max(c.MaxBackups, 0),
		MaxAge:     max(c.MaxAge, 0),
		Compress:   c.Compress,
	}
	if _, err := file.Write(nil); err != nil {
		return nil, &FileSetupError{Path: path, Op: "open", Err: err}
	}
	return file, nil
}

// buildHandler 按 c 的格式与级别构造处理器，并挂上固定字段。
// 输入 w：写入目标，非 nil；c：Format=="console" 用文本，否则 JSON，Level 经 parseLevel；attrs：固定字段，可为空。
// 返回：非 nil slog.Handler，带调用位置（AddSource）。不包 ctx 注入，由 install 负责。
func buildHandler(w io.Writer, c Config, attrs []slog.Attr) slog.Handler {
	opts := &slog.HandlerOptions{Level: parseLevel(c.Level), AddSource: true}
	var h slog.Handler
	if c.Format == FormatConsole {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	if len(attrs) > 0 {
		h = h.WithAttrs(attrs)
	}
	return h
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
