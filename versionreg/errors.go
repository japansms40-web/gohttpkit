package versionreg

import "fmt"

// EmptyVersionError Get 收到空版本 ID。
// Registered 是判定当下的已注册版本快照（字典序新切片），便于调用方提示可选值。
// 判定请用 errors.As，不要扫文案。
type EmptyVersionError struct {
	// Registry 注册表名称（New 的 name）。
	Registry string
	// Registered 判定当下已注册版本的副本，按字典序。
	Registered []ID
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "versionreg: empty version <nil>"；
// 否则 → `versionreg[name]: 必须指定版本(已注册: [v1 v2])`。
func (e *EmptyVersionError) Error() string {
	if e == nil {
		return "versionreg: empty version <nil>"
	}
	return fmt.Sprintf("versionreg[%s]: 必须指定版本(已注册: %v)", e.Registry, e.Registered)
}

// UnknownVersionError Get 的版本未注册。
// Requested / Registered 来自同一次读锁快照，不会出现「报未知但列表已含 requested」。
// 判定请用 errors.As。
type UnknownVersionError struct {
	// Registry 注册表名称。
	Registry string
	// Requested 调用方传入的版本。
	Requested ID
	// Registered 判定当下已注册版本的副本，按字典序。
	Registered []ID
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "versionreg: unknown version <nil>"；
// 否则 → `versionreg[name]: 不支持的版本 "v9"(已注册: [v1])`。
func (e *UnknownVersionError) Error() string {
	if e == nil {
		return "versionreg: unknown version <nil>"
	}
	return fmt.Sprintf("versionreg[%s]: 不支持的版本 %q(已注册: %v)", e.Registry, e.Requested, e.Registered)
}

// MissingConfigFieldError Config.Validate 发现必填字段为空。
// Field 是字段名（ID / BaseURL）。判定请用 errors.As。
type MissingConfigFieldError struct {
	// Field 缺失的必填字段名。
	Field string
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "versionreg: missing config field <nil>"；否则 → `ID 必填`。
func (e *MissingConfigFieldError) Error() string {
	if e == nil {
		return "versionreg: missing config field <nil>"
	}
	return fmt.Sprintf("%s 必填", e.Field)
}
