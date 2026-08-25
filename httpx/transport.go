package httpx

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/japansms40-web/gohttpkit/internal/envx"
	"github.com/japansms40-web/gohttpkit/netproxy"
)

// NewTransport 构造一个调优过的 http.Transport 并按需接上代理。
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
	// 响应头超时默认 15s，可经 env 调整；<=0 视为误配并回落默认
	// （设成 0 会关闭该超时，等于丢掉上面说的那道保护，属于危险的静默降级）。
	respHeaderTimeoutMS := envx.Int(envResponseHeaderTimeout, int(defaultResponseHeaderTimeout/time.Millisecond))
	if respHeaderTimeoutMS <= 0 {
		respHeaderTimeoutMS = int(defaultResponseHeaderTimeout / time.Millisecond)
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		// 即使设了自定义 DialContext 或走代理隧道，也强制尝试 ALPN 协商 HTTP/2，
		// 服务端不支持时自动降级 HTTP/1.1。
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: time.Duration(respHeaderTimeoutMS) * time.Millisecond,
		ExpectContinueTimeout: 5 * time.Second,
		DisableCompression:    true,
	}

	if err := netproxy.ApplyProxyToTransport(transport, proxyURL); err != nil {
		return nil, err
	}
	return transport, nil
}
