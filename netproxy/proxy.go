// Package netproxy 提供代理拨号基建：把 socks5 / http / https 代理接到 http.Transport 上，
// 或直接拿到一个 socks5 的 proxy.Dialer 供 TCP 类链路（MQTT / WebSocket / 裸 TCP）使用。
//
// 所有经本包建立的连接都会过一层 traffic.WrapConn，未注入 traffic.Hook 时零开销、零行为变化；
// 注入后即可在 TCP 层拿到真实收发字节（含 TLS 握手与记录层开销），贴近代理商的计费口径。
package netproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/japansms40-web/gohttpkit/logger"
	"github.com/japansms40-web/gohttpkit/traffic"

	"golang.org/x/net/proxy"
	"log/slog"
)

// ApplyProxyToTransport 根据代理 URL 配置 http.Transport。
// 支持以下 scheme（不区分大小写）：
//   - socks5://[user:pass@]host:port —— 通过 SOCKS5 拨号改写 transport.DialContext
//   - http://[user:pass@]host:port —— 使用 transport.Proxy，由标准库建立 CONNECT 隧道
//   - https://[user:pass@]host:port —— 同上，且客户端到代理服务器之间走 TLS
//
// proxyURL 为空字符串视为不配置代理。
func ApplyProxyToTransport(transport *http.Transport, proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("proxy: parse url: %w", err)
	}
	if u.Hostname() == "" || u.Port() == "" {
		return fmt.Errorf("proxy: missing host or port")
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5":
		dialer, err := socks5DialerFromURL(u)
		if err != nil {
			return err
		}
		transport.DialContext = DialContextWithProxy(dialer)
		return nil
	case "http", "https":
		transport.Proxy = http.ProxyURL(u)
		return nil
	default:
		return fmt.Errorf("proxy: unsupported scheme: %s (supported: socks5, http, https)", u.Scheme)
	}
}

// ParseProxyURL parses and validates a SOCKS5 proxy URL.
// HTTP 客户端请使用 ApplyProxyToTransport；本函数保留给需要直接拿到 proxy.Dialer 的场景，
// 例如 MQTT / WebSocket 等基于 TCP 拨号的链路。
//
// Supports formats:
//   - socks5://host:port
//   - socks5://username:password@host:port
func ParseProxyURL(proxyURL string) (proxy.Dialer, error) {
	if proxyURL == "" {
		return nil, nil
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("proxy: parse url: %w", err)
	}

	if strings.ToLower(u.Scheme) != "socks5" {
		return nil, fmt.Errorf("proxy: unsupported scheme for raw dialer: %s, only socks5 is supported (use ApplyProxyToTransport for http/https)", u.Scheme)
	}

	if u.Hostname() == "" || u.Port() == "" {
		return nil, fmt.Errorf("proxy: missing host or port")
	}

	return socks5DialerFromURL(u)
}

// socks5DialerFromURL 从已解析的 URL 构造 SOCKS5 dialer
func socks5DialerFromURL(u *url.URL) (proxy.Dialer, error) {
	address := net.JoinHostPort(u.Hostname(), u.Port())

	var auth *proxy.Auth
	if u.User != nil {
		auth = &proxy.Auth{User: u.User.Username()}
		if password, ok := u.User.Password(); ok {
			auth.Password = password
		}
	}

	return proxy.SOCKS5("tcp", address, auth, proxy.Direct)
}

// DialContextWithProxy returns a DialContext function for use in http.Transport.
//
// golang.org/x/net/proxy 的 SOCKS5 dialer（proxy.SOCKS5 返回的 *socks.Dialer）实现了
// proxy.ContextDialer：其 DialContext 全链路 honor ctx（连代理用 net.Dialer.DialContext、
// SOCKS 握手 connect(ctx) 也吃 ctx）。直接用它，拨号即受 http 请求 ctx / Client.Timeout 控制，
// 对端 hang 时能被打断——无需 goroutine + Timer 守护，消除卡死时的 goroutine 泄漏。
func DialContextWithProxy(proxyDialer proxy.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if proxyDialer == nil {
		return nil // Will use default dialer
	}
	if cd, ok := proxyDialer.(proxy.ContextDialer); ok {
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := cd.DialContext(ctx, network, addr)
			if err != nil {
				return conn, err // 出错不包裹，保持原语义（避免包裹一个上层不会 Close 的残连接）
			}
			return traffic.WrapConn(conn), nil
		}
	}
	// 兜底：dialer 未实现 ContextDialer（x/net SOCKS5 恒实现，理论不会走到）。
	// 退回旧的 goroutine + 60s 守护包装，保证行为不回退。
	return dialContextWithProxyLegacy(proxyDialer)
}

// legacyDialGuardTimeout 是兜底路径里守护 goroutine 的等待上限：ctx 取消后，
// 那个还挂在 Dial 上的 goroutine 最多再被等这么久，超时就放弃（并打一条 warn）。
// 定义成变量而非常量，仅为让测试能把它压到毫秒级——生产路径永远用默认的 60s。
var legacyDialGuardTimeout = 60 * time.Second

// dialContextWithProxyLegacy 是无 ctx Dial 的兜底包装（仅当 dialer 未实现 ContextDialer 时使用）。
// 用 goroutine + select 让 ctx 取消时本路径立即返回；对端完全不响应时 dial goroutine 会泄漏，
// 加兜底 Timer 限制单次泄漏窗口。正常 SOCKS5 走不到这里。
func dialContextWithProxyLegacy(proxyDialer proxy.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		type dialResult struct {
			conn net.Conn
			err  error
		}
		ch := make(chan dialResult, 1)
		go func() {
			c, e := proxyDialer.Dial(network, addr)
			ch <- dialResult{conn: c, err: e}
		}()
		select {
		case r := <-ch:
			if r.err != nil {
				return r.conn, r.err
			}
			return traffic.WrapConn(r.conn), nil
		case <-ctx.Done():
			logCtx := context.WithoutCancel(ctx)
			go func() {
				t := time.NewTimer(legacyDialGuardTimeout)
				defer t.Stop()
				select {
				case r := <-ch:
					if r.conn != nil {
						_ = r.conn.Close()
					}
				case <-t.C:
					logger.Warn(logCtx, "socks5 dial 守护 goroutine 超时退出,可能存在 socks5 库阻塞", slog.String("addr", addr))
				}
			}()
			return nil, ctx.Err()
		}
	}
}
