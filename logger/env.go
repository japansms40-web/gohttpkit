package logger

import (
	"github.com/japansms40-web/gohttpkit/internal/envx"
)

// 环境变量驱动的全局日志配置。包加载时(init)自动应用,目的是让接入方/运维
// 无需改代码,仅靠环境变量即可把日志落盘到固定文件后离线检索。
//
// 变量名 = envx 前缀(默认 "HTTPKIT_") + 下列后缀:
//
//	LOG_FILE    日志文件路径;一旦设置即落盘到该文件(默认 Output=both,stdout 仍保留)
//	LOG_LEVEL   debug / info / warn / error
//	LOG_FORMAT  json / console
//	LOG_OUTPUT  console / file / both(显式覆盖 LOG_FILE 推导出的 both)
//
// 即 HTTPKIT_LOG_FILE / HTTPKIT_LOG_LEVEL / ...；从 insgo 迁移的项目可在 main 最早处
// envx.SetPrefix("INSGO_") 后调用一次 logger.ApplyEnv()，沿用原有的 INSGO_LOG_* 变量。
//
// 不设任何相关变量时 envConfig 返回 ok=false,init 不调用 SetConfig,
// 行为与硬编码惰性默认(console / json / info)完全一致 —— 对下游零惊扰。
const (
	envLogFile   = "LOG_FILE"
	envLogLevel  = "LOG_LEVEL"
	envLogFormat = "LOG_FORMAT"
	envLogOutput = "LOG_OUTPUT"
)

// envConfig 从环境变量构造 Config;无任何相关变量时返回 (Config{}, false)。
// 仅显式设置的字段才偏离默认:Level 默认 info、Format 默认 json、
// Output 默认 console(但设了 FILE 则推导为 both)。
func envConfig() (Config, bool) {
	file := envx.String(envLogFile)
	level := envx.Lower(envLogLevel)
	format := envx.Lower(envLogFormat)
	output := envx.Lower(envLogOutput)

	if file == "" && level == "" && format == "" && output == "" {
		return Config{}, false
	}

	c := Config{
		Level:    "info",
		Format:   "json",
		Output:   "console",
		FilePath: file,
	}
	if level != "" {
		c.Level = level
	}
	if format != "" {
		c.Format = format
	}
	// 设了文件路径就落盘:默认 both(stdout 不丢 + 文件给运维/AI),可被 LOG_OUTPUT 覆盖。
	if file != "" {
		c.Output = "both"
	}
	if output != "" {
		c.Output = output
	}
	return c, true
}

// ApplyEnv 按当前环境变量(与当前 envx 前缀)重新应用一次全局日志配置。
// 返回 true 表示确实读到了配置并已应用；false 表示没有任何相关变量、保持原样。
//
// 用途：改过 envx.SetPrefix 之后让日志配置重新生效（init 时读的是默认前缀）。
func ApplyEnv() bool {
	c, ok := envConfig()
	if ok {
		SetConfig(c)
	}
	return ok
}

// init 包加载时按环境变量应用全局配置;仅 env 存在才覆盖默认。
// SetHandler/SetLogger 注入的 external logger 仍优先(见 active()),不受影响。
func init() {
	ApplyEnv()
}
