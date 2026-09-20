// Package netproxy 提供代理拨号基建：把 socks5 / http / https 代理接到 http.Transport 上，
// 或直接拿到一个 socks5 的 proxy.Dialer 供 TCP 类链路（MQTT / WebSocket / 裸 TCP）使用。
//
// 所有经本包建立的连接都会过一层 traffic.WrapConn，未注入 traffic.Hook 时零开销、零行为变化；
// 注入后即可在 TCP 层拿到真实收发字节（含 TLS 握手与记录层开销），贴近代理商的计费口径。
//
// 配置 / 解析失败是本包类型错误（*UnsupportedProxySchemeError、*InvalidProxyURLError；
// dialer 未实现 proxy.ContextDialer 时 DialContextWithProxy 返回 *UnsupportedDialerError），
// 判定请用 errors.As，不要扫文案。拨号失败原样返回底层错误，重试识别在 httpx.Do。
package netproxy

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/japansms40-web/gohttpkit/traffic"

	"golang.org/x/net/proxy"
)

// proxy.go —— 代理 URL 解析与接到 Transport / 裸 Dialer。
// 单文件包：scheme 分支和拨号兜底都在这里，再拆只会让「解析」和「拨号」来回跳。

// ApplyProxyToTransport 根据代理 URL 配置 http.Transport。
// 给 httpx.NewTransport 或自建 Client 的配置方：socks5 改 DialContext，http/https 改 Proxy。
// 输入：transport 在 proxyURL 非空时必须非 nil（否则是调用方违约，裸 panic，不包成类型错误）；
// proxyURL 为空串表示不配置代理，不改 transport；scheme 不区分大小写。
// 返回：空串 → nil；socks5 / http / https 成功 → nil 并改写对应字段；
// 非法 URL → *InvalidProxyURLError；其它 scheme → *UnsupportedProxySchemeError{RawDialer:false}。
// 例：ApplyProxyToTransport(tr, "socks5://127.0.0.1:1080") → nil，tr.DialContext 被改写；
// ApplyProxyToTransport(tr, "ftp://h:1") → *UnsupportedProxySchemeError{Scheme:"ftp"}。
// 非空 URL + nil transport 保持 panic：这是配置期违约，不是可恢复的解析失败。
func ApplyProxyToTransport(transport *http.Transport, proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	u, err := parseAndValidateProxyURL(proxyURL)
	if err != nil {
		return err
	}
	scheme, ok := parseScheme(u.Scheme)
	if !ok {
		return &UnsupportedProxySchemeError{Scheme: u.Scheme}
	}
	switch scheme {
	case SchemeSOCKS5:
		dialer, err := socks5DialerFromURL(u)
		if err != nil {
			return err
		}
		transport.DialContext = DialContextWithProxy(dialer)
		return nil
	case SchemeHTTP, SchemeHTTPS:
		transport.Proxy = http.ProxyURL(u)
		return nil
	default:
		return &UnsupportedProxySchemeError{Scheme: u.Scheme}
	}
}

// ParseProxyURL 解析并校验 SOCKS5 代理 URL，返回裸 Dialer。
// 给 MQTT / WebSocket / 裸 TCP 等需要直接拿到 proxy.Dialer 的调用方；HTTP 请用 ApplyProxyToTransport。
// 输入：proxyURL 为空串视为未配置；只接受 socks5（大小写不敏感）；必须带 host 和 port。
// 返回：空串 → (nil, nil)；socks5 成功 → (dialer, nil)；
// 非法 URL → (nil, *InvalidProxyURLError)；其它 scheme → (nil, *UnsupportedProxySchemeError{RawDialer:true})。
// 例：ParseProxyURL("socks5://127.0.0.1:1080") → (dialer, nil)；
// ParseProxyURL("http://127.0.0.1:8080") → (nil, *UnsupportedProxySchemeError{RawDialer:true})。
// 空串与非法 URL 用 (nil, nil) vs 非 nil error 区分，不要把空串当成解析失败。
func ParseProxyURL(proxyURL string) (proxy.Dialer, error) {
	if proxyURL == "" {
		return nil, nil
	}

	u, err := parseAndValidateProxyURL(proxyURL)
	if err != nil {
		return nil, err
	}

	if scheme, ok := parseScheme(u.Scheme); !ok || scheme != SchemeSOCKS5 {
		return nil, &UnsupportedProxySchemeError{Scheme: u.Scheme, RawDialer: true}
	}

	return socks5DialerFromURL(u)
}

// parseAndValidateProxyURL 解析 URL 并要求 host+port 齐全。
// 输入 raw：调用方给的代理 URL 原文；不会回写或改调用方字符串。
// 返回：host 与 port 都在 → (*url.URL, nil)；url.Parse 失败 → *InvalidProxyURLError{Reason:InvalidURLReasonParse}；
// Hostname 或 Port 为空 → *InvalidProxyURLError{Reason:InvalidURLReasonMissingHostOrPort}。
// 不把 url.Parse 的底层错误或 raw 留下，避免账密进日志。
func parseAndValidateProxyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, &InvalidProxyURLError{Reason: InvalidURLReasonParse}
	}
	if u.Hostname() == "" || u.Port() == "" {
		return nil, &InvalidProxyURLError{Reason: InvalidURLReasonMissingHostOrPort}
	}
	return u, nil
}

// socks5DialerFromURL 从已解析的 URL 构造 SOCKS5 dialer。
// 输入 u：必须已通过 parseAndValidateProxyURL（Hostname 与 Port 非空）；User 可空。
// 返回：x/net 的 SOCKS5 Dialer；构造失败极少见，原样返回其 error。
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

// DialContextWithProxy 把 proxy.Dialer 收成 http.Transport.DialContext。
// 给自定义 Transport 或 ApplyProxyToTransport 的 socks5 分支；Migration 用户也可直接用。
// 输入 proxyDialer：nil 表示交给标准库默认拨号；非 nil 必须实现 proxy.ContextDialer。
// 返回：nil dialer → nil 函数（标准库默认拨号）；
// 实现了 ContextDialer → honor ctx 的 DialContext，拨号失败原样返回底层 error、成功才 traffic.WrapConn；
// 未实现 ContextDialer → 一个每次调用都返回 *UnsupportedDialerError 的函数（绝不回落成 nil，
// 否则 transport 会静默直连、绕过代理）。
// 例：DialContextWithProxy(nil) → nil；x/net SOCKS5 dialer → 正常 DialContext。
// 不为只实现 Dial 的 dialer 做无 ctx 的赛跑式降级：那既不能被取消、又会在对端不响应时泄漏
// goroutine。x/net 的 SOCKS5 恒实现 ContextDialer，本库自己的路径不会产生 *UnsupportedDialerError。
func DialContextWithProxy(proxyDialer proxy.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if proxyDialer == nil {
		return nil // Will use default dialer
	}
	cd, ok := proxyDialer.(proxy.ContextDialer)
	if !ok {
		// 只实现 Dial（无 ctx）的 dialer 无法被取消；直接失败，且绝不返回 nil（返回 nil 会让
		// transport 回落成直连、静默绕过代理）。
		return func(context.Context, string, string) (net.Conn, error) {
			return nil, &UnsupportedDialerError{}
		}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := cd.DialContext(ctx, network, addr)
		if err != nil {
			return conn, err // 出错不包裹，保持原语义（避免包裹一个上层不会 Close 的残连接）
		}
		return traffic.WrapConn(conn), nil
	}
}
