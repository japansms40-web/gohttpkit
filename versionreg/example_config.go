package versionreg

import "fmt"

// example_config.go —— 一个可直接用、也可照抄改造的版本配置实现。
//
// 它演示的是本包期望的用法形态：配置对象自带 Validate、用 functional options 构建、
// 必填字段在构造函数签名里强制。你的项目大概率需要自己的字段集，
// 照着 Config + Option + New 这三样改就行。

// Config 一份版本配置：标识 + 基础信息 + 按 endpoint 组织的白名单与参数。
type Config struct {
	// ID 版本标识（必填）。
	ID ID
	// BaseURL 该版本使用的基础域名（必填）。不同版本切域名是常见操作。
	BaseURL string
	// UserAgent 该版本的完整 User-Agent。
	UserAgent string
	// Params 版本级的杂项参数（app id、构建号、协议修订号等），按需取用。
	Params map[string]string

	whitelists *HeaderWhitelists
	// docIDs 按 endpoint 的接口标识（GraphQL doc id、接口版本号等）。
	docIDs map[Endpoint]string
}

// VersionID 实现 Versioned。
func (c *Config) VersionID() ID { return c.ID }

// Validate 实现 Versioned：必填字段缺一不可。
func (c *Config) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("ID 必填")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("BaseURL 必填")
	}
	return nil
}

// HeaderWhitelist 返回某 endpoint 的头白名单副本（未配置返回 nil，即「发全量头」）。
func (c *Config) HeaderWhitelist(ep Endpoint) map[string]string { return c.whitelists.For(ep) }

// DocID 返回某 endpoint 的接口标识；未配置返回空串。
func (c *Config) DocID(ep Endpoint) string {
	if c.docIDs == nil {
		return ""
	}
	return c.docIDs[ep]
}

// Param 返回版本级参数；未配置返回空串。
func (c *Config) Param(key string) string {
	if c.Params == nil {
		return ""
	}
	return c.Params[key]
}

// Option 构建 Config 的可选项。
type Option func(*Config)

// WithUserAgent 设置 User-Agent。
func WithUserAgent(ua string) Option { return func(c *Config) { c.UserAgent = ua } }

// WithParams 设置版本级参数（合并进已有参数）。
func WithParams(params map[string]string) Option {
	return func(c *Config) {
		if c.Params == nil {
			c.Params = make(map[string]string, len(params))
		}
		for k, v := range params {
			c.Params[k] = v
		}
	}
}

// WithHeaderWhitelists 设置按 endpoint 的头白名单。
func WithHeaderWhitelists(src map[Endpoint]map[string]string) Option {
	return func(c *Config) { c.whitelists = NewHeaderWhitelists(src) }
}

// WithDocIDs 设置按 endpoint 的接口标识。
func WithDocIDs(src map[Endpoint]string) Option {
	return func(c *Config) {
		if c.docIDs == nil {
			c.docIDs = make(map[Endpoint]string, len(src))
		}
		for k, v := range src {
			c.docIDs[k] = v
		}
	}
}

// NewConfig 构造版本配置：必填字段走参数、可选字段走 Option。
func NewConfig(id ID, baseURL string, opts ...Option) *Config {
	c := &Config{ID: id, BaseURL: baseURL}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
