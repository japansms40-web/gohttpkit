package netproxy

import "fmt"

// InvalidURLReason 代理 URL 校验失败原因。只允许下面两个枚举值。
type InvalidURLReason string

const (
	// InvalidURLReasonParse url.Parse 失败。
	InvalidURLReasonParse InvalidURLReason = "parse"
	// InvalidURLReasonMissingHostOrPort Hostname 或 Port 为空。
	InvalidURLReasonMissingHostOrPort InvalidURLReason = "missing_host_or_port"
)

// UnsupportedProxySchemeError 代理 URL 的 scheme 不被当前入口接受。
// ApplyProxyToTransport 只接受 socks5 / http / https（RawDialer=false）；
// ParseProxyURL 只接受 socks5（RawDialer=true）。
// 判定请用 errors.As，不要扫 Error() 文案；Scheme 是 URL 原文里的 scheme。
type UnsupportedProxySchemeError struct {
	// Scheme 调用方 URL 里的 scheme，未强制小写。
	Scheme string
	// RawDialer 为 true 表示失败发生在 ParseProxyURL（只接受 socks5）。
	RawDialer bool
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "netproxy: unsupported proxy scheme <nil>"；
// RawDialer=false → `proxy: unsupported scheme: ftp (supported: socks5, http, https)`；
// RawDialer=true → `proxy: unsupported scheme for raw dialer: http, only socks5 is supported (use ApplyProxyToTransport for http/https)`。
func (e *UnsupportedProxySchemeError) Error() string {
	if e == nil {
		return "netproxy: unsupported proxy scheme <nil>"
	}
	if e.RawDialer {
		return fmt.Sprintf("proxy: unsupported scheme for raw dialer: %s, only %s is supported (use ApplyProxyToTransport for %s/%s)", e.Scheme, SchemeSOCKS5, SchemeHTTP, SchemeHTTPS)
	}
	return fmt.Sprintf("proxy: unsupported scheme: %s (supported: %s, %s, %s)", e.Scheme, SchemeSOCKS5, SchemeHTTP, SchemeHTTPS)
}

// InvalidProxyURLError 代理 URL 解析失败或缺少 host/port。
// Reason 只允许 InvalidURLReason 枚举。
// 不保存原始 URL 或 url.Parse 底层错误，避免账密进入日志。判定请用 errors.As。
type InvalidProxyURLError struct {
	// Reason 失败原因枚举。
	Reason InvalidURLReason
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "netproxy: invalid proxy url <nil>"；
// parse → "proxy: parse url"；missing_host_or_port → "proxy: missing host or port"；
// 其它 Reason → "proxy: invalid url"。
func (e *InvalidProxyURLError) Error() string {
	if e == nil {
		return "netproxy: invalid proxy url <nil>"
	}
	switch e.Reason {
	case InvalidURLReasonParse:
		return "proxy: parse url"
	case InvalidURLReasonMissingHostOrPort:
		return "proxy: missing host or port"
	default:
		return "proxy: invalid url"
	}
}

// UnsupportedDialerError 传给 DialContextWithProxy 的 proxy.Dialer 未实现 proxy.ContextDialer。
// 本库自己的路径不会产生它：ApplyProxyToTransport / ParseProxyURL 走的 x/net SOCKS5 dialer
// 恒实现 ContextDialer。只有外部直接调用 DialContextWithProxy 并传入一个只实现 Dial 的
// 自定义 dialer 时才会遇到。
// 返回它而不做无 ctx 的赛跑式降级：Dial 没有 ctx 无法被取消，降级会在对端不响应时泄漏
// goroutine，直接失败更诚实；也绝不回落成直连（那会静默绕过代理）。判定请用 errors.As。
type UnsupportedDialerError struct{}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "netproxy: unsupported dialer <nil>"；
// 否则 → "proxy: dialer must implement proxy.ContextDialer"（本类型无字段，文案恒定）。
func (e *UnsupportedDialerError) Error() string {
	if e == nil {
		return "netproxy: unsupported dialer <nil>"
	}
	return "proxy: dialer must implement proxy.ContextDialer"
}
