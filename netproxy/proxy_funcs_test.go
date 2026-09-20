package netproxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/japansms40-web/gohttpkit/traffic"
)

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
	_, _ = tr.DialContext(context.Background(), "tcp", "127.0.0.1:1")
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

func (d *ctxDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
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
	conn, err := dial(context.Background(), "tcp", "1.2.3.4:80")
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
	conn, err := dial(context.Background(), "tcp", "1.2.3.4:80")
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
	ctx, cancel := context.WithCancel(context.Background())
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

func TestInvalidProxyURLError_未知Reason文案(t *testing.T) {
	err := &InvalidProxyURLError{Reason: "not-a-reason"}
	t.Logf("Error() = %q", err.Error())
	if err.Error() != "proxy: invalid url" {
		t.Fatalf("未知 Reason 应回落 generic 文案，得到 %q", err.Error())
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

func (d *dialOnlyDialer) Dial(network, addr string) (net.Conn, error) {
	d.dialCalled = true
	return nil, nil
}

func TestDialContextWithProxy_非ContextDialer返回明确错误(t *testing.T) {
	d := &dialOnlyDialer{}
	dial := DialContextWithProxy(d)
	if dial == nil {
		t.Fatal("非 nil dialer 不该返回 nil 函数（nil 会让 transport 静默直连、绕过代理）")
	}
	conn, err := dial(context.Background(), "tcp", "1.2.3.4:80")
	t.Logf("conn=%v err=%v dialCalled=%v", conn, err, d.dialCalled)
	if conn != nil {
		t.Fatal("未实现 ContextDialer 时不该返回连接")
	}
	var got *UnsupportedDialerError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *UnsupportedDialerError", err, err)
	}
	if d.dialCalled {
		t.Fatal("既然不支持，就不该真的调用底层 Dial")
	}
}
