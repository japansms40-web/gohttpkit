package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/internal/envx"
)

// 环境变量后缀（完整变量名 = envx 前缀 + 后缀，默认前缀 HTTPKIT_）。
// 全部旋钮集中在此，便于 README / .env.example 与代码保持单一真相源。
const (
	envMaxRetries            = "HTTP_MAX_RETRIES"                // 重试上限，默认 3（共 1+3=4 次尝试）
	envRetryBackoffMS        = "HTTP_RETRY_BACKOFF_MS"           // 指数退避基数 ms，默认 200 → 200/400/800
	envRetryMaxBackoffMS     = "HTTP_RETRY_MAX_BACKOFF_MS"       // 单次退避封顶 ms，默认 0=不封顶
	envTimeoutMS             = "HTTP_TIMEOUT_MS"                 // 整请求超时 ms，默认 30000
	envResponseHeaderTimeout = "HTTP_RESPONSE_HEADER_TIMEOUT_MS" // 响应头超时 ms，默认 15000
	envSlowMS                = "HTTP_SLOW_MS"                    // 慢请求告警阈值 ms，默认 0=关闭
)

// 默认值常量。改这些等于改所有接入方的默认行为，动之前先看 characterization 测试。
const (
	defaultMaxRetries            = 3
	defaultRetryBackoff          = 200 * time.Millisecond
	defaultTimeout               = 30 * time.Second
	defaultResponseHeaderTimeout = 15 * time.Second
)

// RetryPolicy 网络层重试策略。零值经 normalized() 补齐为默认值（并允许 env 覆盖）。
//
// 只有 *TransportError（网络发送本身失败）才进入重试判断；HTTP 状态码不触发重试
// —— 「503 该不该重试」是业务语义，请自己写一层拦截器，不要让基建替你决定。
type RetryPolicy struct {
	// MaxRetries 重试次数上限（总尝试次数 = MaxRetries + 1）。负数视为 0。
	MaxRetries int
	// BaseBackoff 指数退避基数：第 n 次退避 = BaseBackoff * 2^n。
	BaseBackoff time.Duration
	// MaxBackoff 单次退避封顶，0 = 不封顶。
	MaxBackoff time.Duration
	// IsRetryable 判断某个网络错误是否值得重试；nil 时用 errors.IsRetryableNetworkError。
	IsRetryable func(error) bool
	// set 标记本策略是否由调用方显式设置（用于区分零值与「显式要求不重试」）。
	set bool
}

// NoRetry 返回一条「不重试」策略（显式设置，不会被默认值/env 覆盖）。
func NoRetry() RetryPolicy {
	return RetryPolicy{MaxRetries: 0, BaseBackoff: defaultRetryBackoff, set: true}
}

// WithRetry 返回一条显式重试策略。
func WithRetry(maxRetries int, baseBackoff, maxBackoff time.Duration) RetryPolicy {
	return RetryPolicy{MaxRetries: maxRetries, BaseBackoff: baseBackoff, MaxBackoff: maxBackoff, set: true}
}

// normalized 补齐缺省字段：调用方显式设置的值优先，未设置的字段读 env，env 缺省用常量。
func (p RetryPolicy) normalized() RetryPolicy {
	out := p
	if !out.set {
		out.MaxRetries = envx.Int(envMaxRetries, defaultMaxRetries)
		out.BaseBackoff = time.Duration(envx.Int(envRetryBackoffMS, int(defaultRetryBackoff/time.Millisecond))) * time.Millisecond
		out.MaxBackoff = time.Duration(envx.Int(envRetryMaxBackoffMS, 0)) * time.Millisecond
	}
	if out.MaxRetries < 0 {
		out.MaxRetries = 0
	}
	if out.BaseBackoff <= 0 {
		out.BaseBackoff = defaultRetryBackoff
	}
	if out.IsRetryable == nil {
		out.IsRetryable = errors.IsRetryableNetworkError
	}
	out.set = true
	return out
}

// Options 构造 Client 的参数。除 Headers 外全部可选。
type Options struct {
	// Headers 请求头提供者（必填）。见 HeaderProvider 文档。
	Headers HeaderProvider

	// ProxyURL 代理地址，支持 socks5:// / http:// / https://（可带 user:pass@）。空 = 直连。
	ProxyURL string

	// Timeout 整请求超时（连接 + 重定向 + 读体全程）。0 = 读 env HTTP_TIMEOUT_MS，默认 30s。
	Timeout time.Duration

	// Transport 自定义 Transport。传了就整个用你的（本库不再做任何调优，也不再接代理，
	// 代理请自己接进去或用 netproxy.ApplyProxyToTransport）。
	Transport *http.Transport

	// Interceptors 拦截器链。nil → DefaultChain()。
	Interceptors Interceptors

	// Retry 重试策略。零值 → 默认策略（可被 env 覆盖）。
	Retry RetryPolicy

	// LogSummaryOnly 置 true 后 2xx/3xx 的 http_transaction 日志只打 6 个摘要字段
	// （4xx/5xx 始终全量）。几百 worker 并发时压日志体积用。
	LogSummaryOnly bool

	// DisableLogBodyTruncation 置 true 关闭日志里的 body 截断（打完整 body，仅诊断用）。
	DisableLogBodyTruncation bool

	// DisableOriginReferer 置 true 后 bridge 不再自动注入 origin / referer 候选头。
	// 默认注入（值仍要经白名单才会真正发出）。
	DisableOriginReferer bool

	// OnResponseHeaders 每次收到响应时被调用，用于「会话状态回写」：把服务端下发的
	// 新 token / Set-Cookie / 路由提示写回你的 HeaderProvider，让下一次请求自动带上。
	//
	// 挂在 Options 而不是某条链上，是因为它是【会话级】而非【链级】的责任：
	// WithChain 派生出的子客户端共享同一份 Options，回写因此不会因为换链而失效
	// —— 漏掉这一步的典型症状是「全程 200，但业务就是不成功」，因为一直在用过期凭证。
	//
	// 无论状态码如何都会被调用：服务端在 4xx/5xx 上照样可能下发有用的新凭证。
	// 回调在请求热路径上执行，实现必须轻量且并发安全。
	OnResponseHeaders func(ctx context.Context, h http.Header)
}

// timeout 返回归一化后的整请求超时。
func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return time.Duration(envx.Int(envTimeoutMS, int(defaultTimeout/time.Millisecond))) * time.Millisecond
}
