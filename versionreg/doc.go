// Package versionreg 提供「按版本隔离的协议实现」这一模式的骨架：一个类型安全的版本注册表，
// 加上按 endpoint 组织的 header 白名单 / 参数表访问器。
//
// 解决的问题：当你在复刻某个持续演进的私有 API 时，不同客户端版本的请求头集合、
// 参数名、接口地址都会变。把它们塞进同一份实现里加 if version >= X 判断，最终会烂成
// 一团谁都不敢动的分支；而每个版本各写一份完全独立的实现，则需要一个注册与分发机制
// —— 就是本包。
//
// 用法（仿 database/sql 的驱动自注册）：
//
//	// versions/v1/config.go
//	func init() { versions.Registry.MustRegister(cfg) }
//
//	// 使用方
//	cfg, err := versions.Registry.Get("v1.2.3")
//
// 各版本包用一行 blank import 接入分发；新增版本不需要改任何已有代码。
// Get / Validate 失败是本包类型错误，判定请用 errors.As，不要扫文案。
//
// 文件结构：
//
//	versionreg/
//	├── doc.go             包文档（本文件）
//	├── endpoints.go       Endpoint 与按 endpoint 组织的 HeaderWhitelists
//	├── errors.go          本包类型错误与 Op / Kind
//	├── example_config.go  可直接用、也可照抄改造的版本配置实现 Config
//	└── registry.go        泛型版本注册表 Registry[T] 与 ID / Versioned
package versionreg
