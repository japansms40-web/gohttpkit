package netproxy

import "strings"

// scheme.go —— 本包认的代理协议枚举。解析 URL 后只拿枚举做分支，
// 禁止在 Apply / Parse 里再写 "socks5" / "http" / "https" 字面量。

// Scheme 本包认的代理协议。
// 给 ApplyProxyToTransport / ParseProxyURL 做分支；未登记的 scheme 不要硬转成本类型。
type Scheme string

const (
	// SchemeSOCKS5 走 DialContext（改写 transport.DialContext 或返回裸 Dialer）。
	SchemeSOCKS5 Scheme = "socks5"
	// SchemeHTTP 走标准库 transport.Proxy（CONNECT）。
	SchemeHTTP Scheme = "http"
	// SchemeHTTPS 同 HTTP，且客户端到代理之间走 TLS。
	SchemeHTTPS Scheme = "https"
)

// String 实现 fmt.Stringer。
// 输入：接收者是枚举本身。
// 返回：URL 里用的小写 scheme；零值是 ""。
func (s Scheme) String() string { return string(s) }

// parseScheme 把 URL scheme 收成枚举。
// 输入 raw：url.URL.Scheme，大小写不敏感，可带空白以外的原文字符。
// 返回：socks5 / http / https → (对应枚举, true)；空或未登记 → ("", false)。
// 例：parseScheme("SOCKS5") → (SchemeSOCKS5, true)；parseScheme("ftp") → ("", false)。
func parseScheme(raw string) (Scheme, bool) {
	switch Scheme(strings.ToLower(raw)) {
	case SchemeSOCKS5:
		return SchemeSOCKS5, true
	case SchemeHTTP:
		return SchemeHTTP, true
	case SchemeHTTPS:
		return SchemeHTTPS, true
	default:
		return "", false
	}
}
