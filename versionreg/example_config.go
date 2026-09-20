package versionreg

// example_config.go —— 一个可直接用、也可照抄改造的版本配置实现。
//
// 它演示的是本包期望的用法形态：配置对象自带 Validate、用 functional options 构建、
// 必填字段在构造函数签名里强制。你的项目大概率需要自己的字段集，
// 照着 Config + Option + New 这三样改就行。

// Config 一份版本配置：标识 + 基础信息 + 按 endpoint 组织的白名单与参数。
// 给接入方在 init 注册；Get 返回的是表内同一指针，注册后勿改导出字段。
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
// 输入：无。
// 返回：c.ID，可为 ""（MustRegister 会再拦一层）。
func (c *Config) VersionID() ID { return c.ID }

// Validate 实现 Versioned：必填字段缺一不可。
// 输入：接收者的 ID / BaseURL。
// 返回：nil；缺 ID / BaseURL → *MissingConfigFieldError{Field}。
// 例：NewConfig("v1", "https://a.example").Validate() → nil；
// NewConfig("", "...").Validate() → *MissingConfigFieldError{Field:"ID"}。
func (c *Config) Validate() error {
	if c.ID == "" {
		return &MissingConfigFieldError{Field: "ID"}
	}
	if c.BaseURL == "" {
		return &MissingConfigFieldError{Field: "BaseURL"}
	}
	return nil
}

// HeaderWhitelist 返回某 endpoint 的头白名单副本。
// 输入：ep 端点标识。
// 返回：未配置 → nil（发全量头）；已配置 → 副本 map，就地改不影响表内。
func (c *Config) HeaderWhitelist(ep Endpoint) map[string]string { return c.whitelists.For(ep) }

// DocID 返回某 endpoint 的接口标识。
// 输入：ep 端点标识。
// 返回：未配置或 map 为 nil → ""。
func (c *Config) DocID(ep Endpoint) string {
	if c.docIDs == nil {
		return ""
	}
	return c.docIDs[ep]
}

// Param 返回版本级参数。
// 输入：key 参数名。
// 返回：未配置或 map 为 nil → ""。
func (c *Config) Param(key string) string {
	if c.Params == nil {
		return ""
	}
	return c.Params[key]
}

// Option 构建 Config 的可选项。
type Option func(*Config)

// WithUserAgent 设置 User-Agent。
// 输入：ua 完整 UA 串，原样写入。
// 返回：可叠加的 Option。
func WithUserAgent(ua string) Option { return func(c *Config) { c.UserAgent = ua } }

// WithParams 合并版本级参数。
// 输入：params 为 nil 不增加任何项；已有同名 key 被覆盖。
// 返回：可叠加的 Option。
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
// 输入：src 会被 NewHeaderWhitelists 深拷贝；nil 得到空集合（与「未设置、发全量头」不同）。
// 返回：可叠加的 Option。
func WithHeaderWhitelists(src map[Endpoint]map[string]string) Option {
	return func(c *Config) { c.whitelists = NewHeaderWhitelists(src) }
}

// WithDocIDs 合并按 endpoint 的接口标识。
// 输入：src 为 nil 不增加任何项。
// 返回：可叠加的 Option。
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
// 输入：id / baseURL 写入对应字段，不在这里 Validate；opts 按顺序叠加。
// 返回：新 *Config，尚未注册。
// 例：NewConfig("v1", "https://a.example", WithUserAgent("ua/1"))。
func NewConfig(id ID, baseURL string, opts ...Option) *Config {
	c := &Config{ID: id, BaseURL: baseURL}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
