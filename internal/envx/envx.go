// Package envx 收敛本库全部环境变量的读取：统一前缀、统一解析、统一「非法值回落默认」语义。
//
// 前缀默认 "HTTPKIT_"，可在进程启动早期（任何 httpx.New / logger 输出之前）经 SetPrefix
// 改成接入方自己的前缀。例如从 insgo 迁移过来的项目可以 envx.SetPrefix("INSGO_")，
// 原有的 INSGO_LOG_LEVEL / INSGO_HTTP_MAX_RETRIES 等运维变量即可原样继续生效。
//
// 为什么集中：环境变量散落在各包里 os.Getenv 时，「本库到底认哪些变量」没有单一真相源，
// 运维只能翻代码。这里集中之后，Keys() 能一次列全，README 与 .env.example 也有据可依。
package envx

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// DefaultPrefix 是环境变量的默认前缀。
const DefaultPrefix = "HTTPKIT_"

var (
	mu     sync.RWMutex
	prefix = DefaultPrefix
)

// SetPrefix 设置环境变量前缀（如 "MYAPP_"）。传空串表示不加前缀（直接用裸名）。
// 应在进程启动早期调用；logger 的 init() 已按当时的前缀读过一次配置，改前缀后
// 如需让日志配置重新生效，再显式调用一次 logger.SetConfig。
func SetPrefix(p string) {
	mu.Lock()
	prefix = p
	mu.Unlock()
}

// Prefix 返回当前前缀。
func Prefix() string {
	mu.RLock()
	defer mu.RUnlock()
	return prefix
}

// Key 返回加了前缀的完整变量名，供文档/诊断输出使用。
func Key(name string) string { return Prefix() + name }

// String 读取字符串变量并 TrimSpace；未设置返回空串。
func String(name string) string {
	return strings.TrimSpace(os.Getenv(Key(name)))
}

// Lower 读取字符串变量并转小写 + TrimSpace；未设置返回空串。
func Lower(name string) string {
	return strings.ToLower(String(name))
}

// Int 读取非负整数变量；未设置 / 非法 / 负数一律回落 def。
//
// 「非法回落」而非报错是刻意的：环境变量是运维旋钮，打错一个字母不应该让进程起不来，
// 但也绝不能让它变成 0（0 对超时类配置意味着「关闭该保护」，是最危险的静默降级）。
func Int(name string, def int) int {
	if v := String(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// Bool 读取布尔变量（1/true/yes/on 为真，大小写不敏感）；未设置返回 def。
func Bool(name string, def bool) bool {
	v := Lower(name)
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
