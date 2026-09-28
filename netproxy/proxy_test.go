package netproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/japansms40-web/gohttpkit/traffic"
)

func TestApplyProxyToTransport_scheme矩阵(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
		check   func(*testing.T, *http.Transport)
	}{
		{name: "空串不配置", url: "", check: func(t *testing.T, tr *http.Transport) {
			if tr.Proxy != nil {
				t.Fatal("空代理不该设置 Proxy")
			}
		}},
		{name: "socks5 改写 DialContext", url: "socks5://127.0.0.1:1080", check: func(t *testing.T, tr *http.Transport) {
			if tr.Proxy != nil || tr.DialContext == nil {
				t.Fatal("socks5 应改写 DialContext 且不设 Proxy")
			}
		}},
		{name: "socks5 带账密", url: "socks5://u:p@127.0.0.1:1080"},
		{name: "http 走 transport.Proxy", url: "http://127.0.0.1:8080", check: func(t *testing.T, tr *http.Transport) {
			if tr.Proxy == nil {
				t.Fatal("http 代理应设置 transport.Proxy")
			}
		}},
		{name: "https 走 transport.Proxy", url: "https://127.0.0.1:8443", check: func(t *testing.T, tr *http.Transport) {
			if tr.Proxy == nil || tr.DialContext != nil {
				t.Fatal("https 应设置 Proxy 且不改 DialContext")
			}
		}},
		{name: "不支持的 scheme", url: "ftp://127.0.0.1:21", wantErr: true},
		{name: "缺端口", url: "socks5://127.0.0.1", wantErr: true},
		{name: "非法 URL", url: "socks5://%zz", wantErr: true},
		{name: "scheme 大写 SOCKS5", url: "SOCKS5://127.0.0.1:1080", check: func(t *testing.T, tr *http.Transport) {
			if tr.DialContext == nil {
				t.Fatal("SOCKS5:// 应改写 DialContext")
			}
		}},
		{name: "IPv6 host", url: "socks5://[::1]:1080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := &http.Transport{}
			err := ApplyProxyToTransport(tr, tc.url)
			t.Logf("url=%q err=%v", tc.url, err)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, tr)
			}
		})
	}
}

func TestParseProxyURL_只接受socks5(t *testing.T) {
	d, err := ParseProxyURL("")
	t.Logf(`"" → dialer=%v err=%v`, d, err)
	if d != nil || err != nil {
		t.Fatal("空串应返回 (nil, nil)")
	}
	d, err = ParseProxyURL("http://127.0.0.1:8080")
	t.Logf("http → dialer=%v err=%v", d, err)
	if err == nil {
		t.Fatal("裸 dialer 入口只支持 socks5，http 应报错")
	}
	d, err = ParseProxyURL("socks5://127.0.0.1:1080")
	t.Logf("socks5 → dialer=%v err=%v", d != nil, err)
	if err != nil || d == nil {
		t.Fatalf("socks5 应可用: dialer=%v err=%v", d, err)
	}
}

func TestDialContextWithProxy_nil时返回nil(t *testing.T) {
	if DialContextWithProxy(nil) != nil {
		t.Fatal("nil dialer 应返回 nil，交由标准库用默认拨号")
	}
}

// TestSOCKS5_端到端拨号并计流量 用一个最小 SOCKS5 服务端做真实拨号，
// 同时验证 traffic.Hook 能在 TCP 层拿到收发字节。
func TestSOCKS5_端到端拨号并计流量(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through-socks5"))
	}))
	defer backend.Close()

	socksAddr := startMiniSOCKS5(t)

	var read, written atomic.Int64
	traffic.SetHook(func(r, w int64) {
		read.Add(r)
		written.Add(w)
	})
	defer traffic.SetHook(nil)

	tr := &http.Transport{}
	if err := ApplyProxyToTransport(tr, "socks5://"+socksAddr); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(backend.URL + "/x")
	if err != nil {
		t.Fatalf("经 socks5 请求失败: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "through-socks5" {
		t.Fatalf("body = %q", body)
	}
	if read.Load() == 0 || written.Load() == 0 {
		t.Fatalf("流量未被计到: read=%d written=%d", read.Load(), written.Load())
	}
}

func TestDialContext_ctx取消时不挂死(t *testing.T) {
	socksAddr := startMiniSOCKS5(t)
	d, err := ParseProxyURL("socks5://" + socksAddr)
	if err != nil {
		t.Fatal(err)
	}
	dial := DialContextWithProxy(d)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // 立即取消
	if _, err := dial(ctx, "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("已取消的 ctx 应立即失败")
	}
}

// startMiniSOCKS5 起一个只支持「无认证 + CONNECT + IPv4/域名」的最小 SOCKS5 服务端。
func startMiniSOCKS5(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSOCKS5(conn)
		}
	}()
	return ln.Addr().String()
}

func serveSOCKS5(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// 握手：VER NMETHODS METHODS...
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil { // 无认证
		return
	}

	// 请求：VER CMD RSV ATYP ADDR PORT
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 0x01: // IPv4
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		host = net.IP(buf).String()
	case 0x03: // 域名
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return
		}
		buf := make([]byte, l[0])
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		host = string(buf)
	default:
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	target := net.JoinHostPort(host, fmt.Sprint(binary.BigEndian.Uint16(portBuf)))

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = upstream.Close() }()
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, conn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, upstream); done <- struct{}{} }()
	<-done
}

func TestApplyProxyToTransport_非空URL且transport为nil会panic(t *testing.T) {
	defer func() {
		r := recover()
		t.Logf("recover=%v", r)
		if r == nil {
			t.Fatal("非空代理 + nil transport 应 panic（调用方违约，不改成类型错误）")
		}
	}()
	_ = ApplyProxyToTransport(nil, "socks5://127.0.0.1:1080")
}

func TestParseProxyURL_错误分支(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"非法 URL", "socks5://%zz"},
		{"缺端口", "socks5://127.0.0.1"},
		{"缺 host", "socks5://:1080"},
		{"非 socks5", "https://127.0.0.1:8443"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ParseProxyURL(tc.url)
			t.Logf("url=%q dialer=%v err=%v", tc.url, d, err)
			if err == nil {
				t.Fatalf("%q 应报错", tc.url)
			}
			if d != nil {
				t.Fatal("失败时 dialer 必须为 nil")
			}
		})
	}
}

func TestApplyProxyToTransport_缺host也报错(t *testing.T) {
	if err := ApplyProxyToTransport(&http.Transport{}, "socks5://:1080"); err == nil {
		t.Fatal("缺 host 应报错")
	}
}

func TestParseAndValidateProxyURL_表驱动(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantHost   string
		wantPort   string
		wantReason InvalidURLReason
	}{
		{"host和port齐全", "socks5://127.0.0.1:1080", "127.0.0.1", "1080", ""},
		{"IPv6", "socks5://[::1]:1080", "::1", "1080", ""},
		{"仅用户名无密码", "socks5://onlyuser@127.0.0.1:1080", "127.0.0.1", "1080", ""},
		{"带账密", "socks5://u:p@127.0.0.1:1080", "127.0.0.1", "1080", ""},
		{"http 同样校验", "http://127.0.0.1:8080", "127.0.0.1", "8080", ""},
		{"空串缺 host/port", "", "", "", InvalidURLReasonMissingHostOrPort},
		{"非法转义", "socks5://%zz", "", "", InvalidURLReasonParse},
		{"缺端口", "socks5://127.0.0.1", "", "", InvalidURLReasonMissingHostOrPort},
		{"缺host", "socks5://:1080", "", "", InvalidURLReasonMissingHostOrPort},
		{"只有 scheme", "socks5://", "", "", InvalidURLReasonMissingHostOrPort},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := parseAndValidateProxyURL(tc.raw)
			t.Logf("raw=%q → host=%q port=%q err=%v", tc.raw, hostnameOfURL(u), portOfURL(u), err)
			if tc.wantReason != "" {
				assertInvalidProxyURL(t, err, tc.wantReason)
				if u != nil {
					t.Fatal("失败时 URL 必须为 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err=%v", err)
			}
			if u.Hostname() != tc.wantHost || u.Port() != tc.wantPort {
				t.Fatalf("host:port = %s:%s, want %s:%s", u.Hostname(), u.Port(), tc.wantHost, tc.wantPort)
			}
		})
	}
}

func hostnameOfURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Hostname()
}

func portOfURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Port()
}

func TestSocks5DialerFromURL_用户名密码组合(t *testing.T) {
	cases := []string{
		"socks5://127.0.0.1:1080",
		"socks5://onlyuser@127.0.0.1:1080",
		"socks5://u:p@127.0.0.1:1080",
		"socks5://[::1]:1080",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			u, err := parseAndValidateProxyURL(raw)
			if err != nil {
				t.Fatal(err)
			}
			d, err := socks5DialerFromURL(u)
			t.Logf("raw=%q dialer=%T err=%v", raw, d, err)
			if err != nil || d == nil {
				t.Fatalf("socks5DialerFromURL = (%v, %v)", d, err)
			}
		})
	}
}

func TestApplyProxyToTransport_socks5只改DialContext(t *testing.T) {
	tr := &http.Transport{}
	err := ApplyProxyToTransport(tr, "socks5://127.0.0.1:1080")
	t.Logf("err=%v Proxy_nil=%v DialContext_nil=%v", err, tr.Proxy == nil, tr.DialContext == nil)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Proxy != nil {
		t.Fatal("socks5 不该设置 transport.Proxy")
	}
	if tr.DialContext == nil {
		t.Fatal("socks5 必须改写 DialContext")
	}
}

func TestApplyProxyToTransport_http代理可从Transport取回(t *testing.T) {
	tr := &http.Transport{}
	err := ApplyProxyToTransport(tr, "http://u:p@127.0.0.1:8080")
	t.Logf("err=%v Proxy_nil=%v", err, tr.Proxy == nil)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Proxy == nil {
		t.Fatal("http 必须设置 transport.Proxy")
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := tr.Proxy(req)
	t.Logf("Proxy(req) → %v err=%v", u, err)
	if err != nil || u == nil {
		t.Fatalf("取回代理 URL 失败: %v %v", u, err)
	}
	if u.Scheme != string(SchemeHTTP) || u.Host != "127.0.0.1:8080" {
		t.Fatalf("代理 URL = %s, want http://127.0.0.1:8080", u)
	}
	pass, ok := u.User.Password()
	if u.User.Username() != "u" || !ok || pass != "p" {
		t.Fatalf("账密 = %q / %q", u.User.Username(), pass)
	}
	if tr.DialContext != nil {
		t.Fatal("http 不该改写 DialContext")
	}
}

func TestApplyProxyToTransport_https同样走Proxy(t *testing.T) {
	tr := &http.Transport{}
	err := ApplyProxyToTransport(tr, "HTTPS://127.0.0.1:8443")
	t.Logf("err=%v Proxy_nil=%v", err, tr.Proxy == nil)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	u, err := tr.Proxy(req)
	t.Logf("Proxy(req) → %v err=%v", u, err)
	if err != nil || u == nil || u.Scheme != "https" || u.Host != "127.0.0.1:8443" {
		t.Fatalf("https 代理 URL = %v err=%v", u, err)
	}
}

func TestApplyProxyToTransport_空串不改已有字段(t *testing.T) {
	tr := &http.Transport{}
	called := false
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		called = true
		return nil, errors.New("sentinel")
	}
	if err := ApplyProxyToTransport(tr, ""); err != nil {
		t.Fatal(err)
	}
	t.Logf("空串后 DialContext 仍在=%v", tr.DialContext != nil)
	if tr.DialContext == nil || tr.Proxy != nil {
		t.Fatal("空串不得清掉已有 DialContext，也不得设置 Proxy")
	}
	_, _ = tr.DialContext(t.Context(), "tcp", "127.0.0.1:1")
	if !called {
		t.Fatal("原来的 DialContext 应还能被调用")
	}
}

func TestParseProxyURL_仅用户名无密码(t *testing.T) {
	d, err := ParseProxyURL("socks5://onlyuser@127.0.0.1:1080")
	t.Logf("dialer=%T err=%v", d, err)
	if err != nil || d == nil {
		t.Fatalf("仅用户名应可用: %v %v", d, err)
	}
}

func TestParseProxyURL_ftp是裸DialerUnsupported(t *testing.T) {
	d, err := ParseProxyURL("ftp://127.0.0.1:21")
	t.Logf("dialer=%v err=%v", d, err)
	if d != nil {
		t.Fatal("失败时 dialer 必须为 nil")
	}
	assertUnsupportedScheme(t, err, "ftp", true)
}

type ctxDialer struct {
	conn net.Conn
	err  error
}

func (d *ctxDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

func (d *ctxDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d.conn, d.err
}

func TestDialContextWithProxy_ContextDialer成功才包流量(t *testing.T) {
	traffic.SetHook(func(r, w int64) {})
	defer traffic.SetHook(nil)

	backend := newPipeConn()
	t.Cleanup(func() { _ = backend.Close() })
	dial := DialContextWithProxy(&ctxDialer{conn: backend})
	conn, err := dial(t.Context(), "tcp", "1.2.3.4:80")
	t.Logf("conn=%T err=%v same=%v", conn, err, conn == net.Conn(backend))
	if err != nil {
		t.Fatal(err)
	}
	if conn == net.Conn(backend) {
		t.Fatal("成功路径必须 traffic.WrapConn，不能返回裸 conn")
	}
}

func TestDialContextWithProxy_ContextDialer失败原样返回(t *testing.T) {
	sentinel := errors.New("ctx dial refused")
	leftover := newPipeConn()
	t.Cleanup(func() { _ = leftover.Close() })
	dial := DialContextWithProxy(&ctxDialer{conn: leftover, err: sentinel})
	conn, err := dial(t.Context(), "tcp", "1.2.3.4:80")
	t.Logf("conn=%T err=%v", conn, err)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 原样返回底层错误", err)
	}
	if conn != net.Conn(leftover) {
		t.Fatal("失败时必须原样返回底层 conn，不得 WrapConn")
	}
}

func TestDialContextWithProxy_ContextDialer尊重已取消ctx(t *testing.T) {
	dial := DialContextWithProxy(&ctxDialer{conn: newPipeConn()})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	conn, err := dial(ctx, "tcp", "1.2.3.4:80")
	t.Logf("conn=%v err=%v", conn, err)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if conn != nil {
		t.Fatal("已取消时不该返回连接")
	}
}

// pipeConn 是一端可关闭的假连接（另一端在 newPipeConn 里立即关闭），
// DialContextWithProxy 相关用例把它当作 backend conn 用。
type pipeConn struct {
	net.Conn
	closed chan struct{}
}

func newPipeConn() *pipeConn {
	c1, c2 := net.Pipe()
	go func() { _ = c2.Close() }()
	return &pipeConn{Conn: c1, closed: make(chan struct{}, 1)}
}

func (c *pipeConn) Close() error {
	select {
	case c.closed <- struct{}{}:
	default:
	}
	return c.Conn.Close()
}

// dialOnlyDialer 只实现 proxy.Dialer（Dial），不实现 proxy.ContextDialer，
// 用来触发 DialContextWithProxy 的 *UnsupportedDialerError 分支。
type dialOnlyDialer struct {
	dialCalled bool
}

func (d *dialOnlyDialer) Dial(_, _ string) (net.Conn, error) {
	d.dialCalled = true
	return nil, nil
}

func TestDialContextWithProxy_非ContextDialer返回明确错误(t *testing.T) {
	d := &dialOnlyDialer{}
	dial := DialContextWithProxy(d)
	if dial == nil {
		t.Fatal("非 nil dialer 不该返回 nil 函数（nil 会让 transport 静默直连、绕过代理）")
	}
	conn, err := dial(t.Context(), "tcp", "1.2.3.4:80")
	t.Logf("conn=%v err=%v dialCalled=%v", conn, err, d.dialCalled)
	if conn != nil {
		t.Fatal("未实现 ContextDialer 时不该返回连接")
	}
	if _, ok := errors.AsType[*UnsupportedDialerError](err); !ok {
		t.Fatalf("err = %v (%T), want *UnsupportedDialerError", err, err)
	}
	if d.dialCalled {
		t.Fatal("既然不支持，就不该真的调用底层 Dial")
	}
}

func FuzzParseProxyURL(f *testing.F) {
	f.Add("socks5://127.0.0.1:1080")
	f.Add("socks5://u:p@host.example:1080")
	f.Add("")
	f.Add("http://x")
	f.Add(":")
	f.Fuzz(func(t *testing.T, raw string) {
		d, err := ParseProxyURL(raw)
		if err != nil && d != nil {
			t.Fatalf("err != nil 时 dialer 必须为 nil, raw=%q dialer=%T err=%v", raw, d, err)
		}
	})
}
