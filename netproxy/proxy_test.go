package netproxy_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/japansms40-web/gohttpkit/netproxy"
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
			err := netproxy.ApplyProxyToTransport(tr, tc.url)
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
	d, err := netproxy.ParseProxyURL("")
	t.Logf(`"" → dialer=%v err=%v`, d, err)
	if d != nil || err != nil {
		t.Fatal("空串应返回 (nil, nil)")
	}
	d, err = netproxy.ParseProxyURL("http://127.0.0.1:8080")
	t.Logf("http → dialer=%v err=%v", d, err)
	if err == nil {
		t.Fatal("裸 dialer 入口只支持 socks5，http 应报错")
	}
	d, err = netproxy.ParseProxyURL("socks5://127.0.0.1:1080")
	t.Logf("socks5 → dialer=%v err=%v", d != nil, err)
	if err != nil || d == nil {
		t.Fatalf("socks5 应可用: dialer=%v err=%v", d, err)
	}
}

func TestDialContextWithProxy_nil时返回nil(t *testing.T) {
	if netproxy.DialContextWithProxy(nil) != nil {
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
	if err := netproxy.ApplyProxyToTransport(tr, "socks5://"+socksAddr); err != nil {
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
	d, err := netproxy.ParseProxyURL("socks5://" + socksAddr)
	if err != nil {
		t.Fatal(err)
	}
	dial := netproxy.DialContextWithProxy(d)

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

func TestApplyProxyToTransport_ftp是UnsupportedScheme(t *testing.T) {
	err := netproxy.ApplyProxyToTransport(&http.Transport{}, "ftp://h:1")
	t.Logf("ftp → err=%v", err)
	var got *netproxy.UnsupportedProxySchemeError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *UnsupportedProxySchemeError", err, err)
	}
	if got.Scheme != "ftp" || got.RawDialer {
		t.Fatalf("Scheme=%q RawDialer=%v, want ftp / false", got.Scheme, got.RawDialer)
	}
}

func TestApplyProxyToTransport_非空URL且transport为nil会panic(t *testing.T) {
	defer func() {
		r := recover()
		t.Logf("recover=%v", r)
		if r == nil {
			t.Fatal("非空代理 + nil transport 应 panic（调用方违约，不改成类型错误）")
		}
	}()
	_ = netproxy.ApplyProxyToTransport(nil, "socks5://127.0.0.1:1080")
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
			d, err := netproxy.ParseProxyURL(tc.url)
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
	if err := netproxy.ApplyProxyToTransport(&http.Transport{}, "socks5://:1080"); err == nil {
		t.Fatal("缺 host 应报错")
	}
}
