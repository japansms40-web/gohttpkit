package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/japansms40-web/gohttpkit/errors"
)

// options.go —— Options / RetryPolicy 与默认值。
// 独立成文件：所有「改了会影响每个接入方」的旋钮集中在此，和 Client 入口、拦截器实现分开。
// 超时 / 重试 / 慢请求只认 Options，不读环境变量。

// 默认值常量。改这些等于改所有接入方的默认行为，动之前先看 characterization 测试。
const (
	defaultMaxRetries            = 3
	defaultRetryBackoff          = 200 * time.Millisecond
	defaultTimeout               = 30 * time.Second
	defaultResponseHeaderTimeout = 15 * time.Second

	// transport 连接池与握手超时。与 defaultTimeout 同源的拨号超时不要再写 30s 字面量。
	defaultMaxIdleConns          = 10
	defaultMaxIdleConnsPerHost   = 5
	defaultIdleConnTimeout       = 90 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultExpectContinueTimeout = 5 * time.Second
)

// RetryPolicy 网络层重试策略。
//
// 只有 *TransportError（网络发送本身失败）才进入重试判断；HTTP 状态码不触发重试
// —— 「503 该不该重试」是业务语义，请自己写一层拦截器，不要让基建替你决定。
//
// 在 Options 里以【指针】出现：nil 表示「没配」（走代码默认值），
// 非 nil 表示「配了」，此时每个字段都字面生效 —— 包括 MaxRetries: 0（就是不重试）。
// 用指针而不是「零值即未配」，是因为后者分不清「没配」和「明确要求不重试」。
type RetryPolicy struct {
	// MaxRetries 重试次数上限（总尝试次数 = MaxRetries + 1）。负数按 0 处理。
	MaxRetries int
	// BaseBackoff 指数退避基数：第 n 次退避 = BaseBackoff * 2^n。<=0 回落默认 200ms。
	BaseBackoff time.Duration
	// MaxBackoff 单次退避封顶，0 = 不封顶。
	MaxBackoff time.Duration
	// IsRetryable 判断某个网络错误是否值得重试；nil 时用 errors.IsRetryableNetworkError。
	IsRetryable func(error) bool
}

// NoRetry 返回一条「不重试」策略，可直接赋给 Options.Retry。
// 给明确不要网络重试的调用方（幂等性未知、或上层自己重试）。
// 输入：无。
// 返回：MaxRetries=0 的策略；BaseBackoff 已填默认 200ms。
// 例：Options{Retry: NoRetry()} 构造后总尝试次数为 1。
func NoRetry() *RetryPolicy {
	return &RetryPolicy{MaxRetries: 0, BaseBackoff: defaultRetryBackoff}
}

// WithRetry 返回一条显式重试策略，可直接赋给 Options.Retry。
// 给不想手写 RetryPolicy 字面量的调用方。
// 输入 maxRetries：重试次数上限（总尝试 = 该值 + 1）；负数在 New 时纠正为 0。
// 输入 baseBackoff / maxBackoff：指数退避基数与单次封顶；baseBackoff<=0 在 New 时回落 200ms。
// 返回：字面填好的 *RetryPolicy，IsRetryable 仍为 nil（New 时补默认实现）。
// 例：WithRetry(2, 100*time.Millisecond, time.Second) → 最多 3 次尝试。
func WithRetry(maxRetries int, baseBackoff, maxBackoff time.Duration) *RetryPolicy {
	return &RetryPolicy{MaxRetries: maxRetries, BaseBackoff: baseBackoff, MaxBackoff: maxBackoff}
}

// normalizeRetry 归一化重试策略。
// 给 New：把指针选项收成 Client 热路径上的值类型。
// 输入 p：nil 走代码默认值（3 次 + 200ms）；非 nil 字面采用并补齐非法值。
// 返回：可直接赋给 Client.retry 的值类型；IsRetryable 保证非 nil；不改入参。
func normalizeRetry(p *RetryPolicy) RetryPolicy {
	var out RetryPolicy
	if p == nil {
		out = RetryPolicy{
			MaxRetries:  defaultMaxRetries,
			BaseBackoff: defaultRetryBackoff,
		}
	} else {
		out = *p
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
	return out
}

// Options 构造 Client 的参数。除 Headers 外全部可选。
type Options struct {
	// Headers 请求头提供者（必填）。见 HeaderProvider 文档。
	Headers HeaderProvider

	// ProxyURL 代理地址，支持 socks5:// / http:// / https://（可带 user:pass@）。空 = 直连。
	ProxyURL string

	// ExitIP 代理出口公网 IP。仅写入 Transaction / 日志，本库不校验、不探测。
	ExitIP string

	// ASN 代理出口 ASN（如 "AS64500" 或 "64500"）。仅写入 Transaction / 日志，本库不校验、不探测。
	ASN string

	// Timeout 整请求超时（连接 + 重定向 + 读体全程）。0 = 默认 30s。
	Timeout time.Duration

	// ResponseHeaderTimeout 等响应头的超时。0 / 负数回落 15s。
	// 设成 0 会关掉这道保护（HTTP/2 下整请求 Timeout 拦不住卡头），禁止当「关闭」用。
	// 传了自定义 Transport 时本字段不生效，请自己设 transport.ResponseHeaderTimeout。
	ResponseHeaderTimeout time.Duration

	// Transport 自定义 Transport。传了就整个用你的（本库不再做任何调优，也不再接代理，
	// 代理请自己接进去或用 netproxy.ApplyProxyToTransport）。
	Transport *http.Transport

	// Interceptors 拦截器链。nil / 空切片都原样收下，不会装默认链。
	// 开箱即用请走 httpx.NewClient，它只在本字段为 nil 时填注册的默认链（import httpx/interceptor 即注册）。
	Interceptors Interceptors

	// Retry 重试策略。nil → 默认策略（3 次 + 200/400/800ms）；
	// 非 nil 则字面生效。用 NoRetry() 关掉重试，WithRetry(...) 自定义。
	Retry *RetryPolicy

	// LogSummaryOnly 置 true 后 2xx/3xx 的 event=http.transaction 日志只打 6 个摘要字段
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

	// SlowMS 慢请求告警阈值。>0 时 event=http.transaction 超阈值降为 Warn 并标 slow=true。
	// 0 = 关闭。在 New 时固化。字段名是公开契约，单位写在注释里，不能改成 Slow。
	//nolint:staticcheck // ST1011：公开字段名 SlowMS 已是契约，改名会破坏调用方。
	SlowMS time.Duration
}

// timeout 返回归一化后的整请求超时。
// 给 New：写进 http.Client.Timeout。
// 输入：Options.Timeout；<=0 视为未配，不表示「关闭超时」。
// 返回：>0 原样返回，否则默认 30s。
func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return defaultTimeout
}

// responseHeaderTimeout 返回归一化后的响应头超时。
// 给 newTransport：写进 Transport.ResponseHeaderTimeout。
// 输入：Options.ResponseHeaderTimeout；<=0 视为未配或误配。
// 返回：>0 原样返回，否则默认 15s（0 会关掉保护，禁止当关闭用）。
func (o Options) responseHeaderTimeout() time.Duration {
	if o.ResponseHeaderTimeout > 0 {
		return o.ResponseHeaderTimeout
	}
	return defaultResponseHeaderTimeout
}

// slowMS 返回归一化后的慢请求阈值（毫秒）。
// 给 New：固化到 Client.slowMS，热路径不再回读 Options。
// 输入：Options.SlowMS；<=0 视为关闭。
// 返回：毫秒整数；0 = 关闭慢请求告警。
func (o Options) slowMS() int {
	if o.SlowMS > 0 {
		return int(o.SlowMS / time.Millisecond)
	}
	return 0
}
