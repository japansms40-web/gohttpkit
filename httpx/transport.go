package httpx

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/japansms40-web/gohttpkit/netproxy"
)

// transport.go —— 调优过的 http.Transport 构造。独立成文件：每条超时/连接池参数
// 都带着生产踩坑注释，和 Options 默认值同文件会把「旋钮」与「为什么是这个数」搅在一起。

// NewTransport 构造一个调优过的 http.Transport 并按需接上代理。
// 给自建 http.Client 或不走 httpx.New 的调用方。
// 输入 proxyURL：空 = 直连；socks5 / http / https。
// 返回：调优过的 *http.Transport，ResponseHeaderTimeout 固定 15s；
// 代理失败是 netproxy 类型（*UnsupportedProxySchemeError / *InvalidProxyURLError），
// 经 httpx.New 时还会再包一层 "httpx: build transport: %w"。
// 例：NewTransport("") → 直连 Transport；NewTransport("ftp://h") → *UnsupportedProxySchemeError。
// 要改响应头超时请走 Options.ResponseHeaderTimeout，或拿到返回值后自己改字段。
//
// 这里每一条参数都是踩过坑之后加的，改之前请先读注释：
//   - idle 连接上限：单条 idle conn 持有的 socket buffer 可达 15~20MB。不设上限 + 不设
//     超时的话，业务侧「销毁-重建 Client」的循环会让 idle TCP/TLS 持续累积，最终撑爆
//     容器内存限制。
//   - ResponseHeaderTimeout：慢服务端只发几百字节响应头就卡住时，http.Client.Timeout
//     对 HTTP/2 并不完全生效，单个 in-flight 请求可能挂在 read header 上好几分钟。
//     这是 socket buffer 堆积的主因之一，必须单独设。
//   - DisableCompression：本库自己按 content-encoding 解压（zstd/gzip/deflate/br），
//     关掉标准库的自动 gzip，避免双重解压与重复内存分配；也让请求头里的
//     accept-encoding 完全由 HeaderProvider 说了算（它是指纹的一部分）。
func NewTransport(proxyURL string) (*http.Transport, error) {
	return newTransport(proxyURL, defaultResponseHeaderTimeout)
}

// newTransport 与 NewTransport 相同，但响应头超时由调用方传入。
// 给 New / NewTransport：唯一真正构造 Transport 的地方。
// 输入 proxyURL：同 NewTransport，空串直连。
// 输入 headerTimeout：<=0 回落 15s（0 会关掉保护，禁止当关闭用）。
// 返回：调优过的 *http.Transport；代理失败原样返回 netproxy 类型，不在这里再包一层。
func newTransport(proxyURL string, headerTimeout time.Duration) (*http.Transport, error) {
	if headerTimeout <= 0 {
		headerTimeout = defaultResponseHeaderTimeout
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: defaultTimeout,
		}).DialContext,
		// 即使设了自定义 DialContext 或走代理隧道，也强制尝试 ALPN 协商 HTTP/2，
		// 服务端不支持时自动降级 HTTP/1.1。
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          defaultMaxIdleConns,
		MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: headerTimeout,
		ExpectContinueTimeout: defaultExpectContinueTimeout,
		DisableCompression:    true,
	}

	if err := netproxy.ApplyProxyToTransport(transport, proxyURL); err != nil {
		return nil, err
	}
	return transport, nil
}
