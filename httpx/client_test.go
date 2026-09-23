package httpx_test

// client_test.go —— Client 构造访问器与 Do 入口的单元契约。

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/netproxy"
)

// ─────────────────────────────── client.go ───────────────────────────────

func TestPostJSON_自动带contentType(t *testing.T) {
	var gotCT, gotBody string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.PostJSON(context.Background(), "/x", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	gotBody = string(srv.bodies[len(srv.bodies)-1])
	srv.mu <- struct{}{}
	t.Logf("content-type=%q body=%q", gotCT, gotBody)
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if gotBody != `{"n":1}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestClient_访问器(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	hp := httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"accept": "*/*"}}
	c, err := httpx.NewClient(httpx.Options{Headers: hp, ProxyURL: "", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("BaseURL=%q interceptors=%d timeout=%v retries=%d", c.Headers().BaseURL(), len(c.Interceptors()), c.HTTPClient.Timeout, c.RetryPolicy().MaxRetries)
	if c.Headers().BaseURL() != srv.URL {
		t.Fatal("Headers() 不对")
	}
	if got := len(c.Interceptors()); got != len(interceptor.DefaultChain()) {
		t.Fatalf("Interceptors() 长度 = %d", got)
	}
	// 返回的是副本：改它不该影响客户端
	chain := c.Interceptors()
	chain[0] = nil
	if c.Interceptors()[0] == nil {
		t.Fatal("Interceptors() 返回了内部切片")
	}
	if c.Options().Timeout != 5*time.Second {
		t.Fatal("Options() 不对")
	}
	if c.RetryPolicy().MaxRetries != 3 {
		t.Fatalf("默认重试次数 = %d, want 3", c.RetryPolicy().MaxRetries)
	}
	if c.HTTPClient.Timeout != 5*time.Second {
		t.Fatalf("Timeout 未生效: %v", c.HTTPClient.Timeout)
	}
	c.SetTimeout(time.Second)
	t.Logf("SetTimeout → %v", c.HTTPClient.Timeout)
	if c.HTTPClient.Timeout != time.Second {
		t.Fatal("SetTimeout 未生效")
	}
}

func TestClient_SlowMS固化阈值(t *testing.T) {
	var nilClient *httpx.Client
	t.Logf("nil SlowMS=%d", nilClient.SlowMS())
	if got := nilClient.SlowMS(); got != 0 {
		t.Fatalf("nil client SlowMS = %d, want 0", got)
	}

	c, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		SlowMS:  1500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SlowMS=%d", c.SlowMS())
	if got := c.SlowMS(); got != 1500 {
		t.Fatalf("SlowMS = %d, want 1500", got)
	}
}

func TestClient_CacheStatusCode与响应头(t *testing.T) {
	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
	if err != nil {
		t.Fatal(err)
	}
	c.CacheStatusCode(404)
	h := make(http.Header)
	h.Set("content-type", "text/html")
	c.CacheResponseHeaders(h)
	t.Logf("status=%d ct=%q", c.SnapshotResponseStatusCode(), c.SnapshotResponseHeaders().Get("content-type"))
	if got := c.SnapshotResponseStatusCode(); got != 404 {
		t.Fatalf("status = %d, want 404", got)
	}
	if got := c.SnapshotResponseHeaders().Get("content-type"); got != "text/html" {
		t.Fatalf("header = %q", got)
	}
}

func TestLogBodyLimit_三种情形(t *testing.T) {
	var nilClient *httpx.Client
	t.Logf("nil=%d default=%d truncatedOff=%d", nilClient.LogBodyLimit(), httpx.LogBodyMaxBytes, 0)
	if got := nilClient.LogBodyLimit(); got != httpx.LogBodyMaxBytes {
		t.Fatalf("nil client = %d, want 回落默认(截断开启)", got)
	}

	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("default client limit=%d", c.LogBodyLimit())
	if got := c.LogBodyLimit(); got != httpx.LogBodyMaxBytes {
		t.Fatalf("默认 = %d", got)
	}

	c2, err := httpx.New(httpx.Options{
		Headers:                  httpx.StaticHeaders{Base: "https://x.example"},
		DisableLogBodyTruncation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("DisableLogBodyTruncation limit=%d", c2.LogBodyLimit())
	if got := c2.LogBodyLimit(); got != 0 {
		t.Fatalf("关闭截断时 = %d, want 0", got)
	}
}

func TestDo_body编码失败时不发请求(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(context.Background(), httpx.RequestSpec{Method: http.MethodPost, Path: "/x", Body: make(chan int)})
	t.Logf("encode fail err=%v (%T) served=%d", err, err, srv.count())
	if err == nil {
		t.Fatal("want error")
	}
	if srv.count() != 0 {
		t.Fatal("编码失败不该发出网络请求")
	}
}

func TestDo_空Method默认GET(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	if _, err := c.Do(context.Background(), httpx.RequestSpec{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	t.Logf("empty method → %q", srv.last().Method)
	if got := srv.last().Method; got != http.MethodGet {
		t.Fatalf("method = %q", got)
	}
}

func TestDo_空链不再回落默认链(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c, err := httpx.New(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Interceptors: httpx.Interceptors{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/x", nil)
	t.Logf("empty chain err=%v", err)
	assertChainExhausted(t, err, 0, 0)
}

func TestNew_缺HeaderProvider报错(t *testing.T) {
	_, err := httpx.New(httpx.Options{})
	assertMissingHeaderProvider(t, err, "Options.Headers")
}

func TestNew_非法代理URL仍可As到netproxy类型(t *testing.T) {
	_, err := httpx.New(httpx.Options{
		Headers:  httpx.StaticHeaders{Base: "https://x.example"},
		ProxyURL: "ftp://1.2.3.4:1080",
	})
	if err == nil {
		t.Fatal("不支持的代理 scheme 应在构造期报错")
	}
	if !strings.HasPrefix(err.Error(), "httpx: build transport:") {
		t.Fatalf("err = %v, want 保留 build transport 前缀", err)
	}
	var ue *netproxy.UnsupportedProxySchemeError
	if !errors.As(err, &ue) || ue.Scheme != "ftp" {
		t.Fatalf("err = %v (%T), want *netproxy.UnsupportedProxySchemeError Scheme=ftp", err, err)
	}
	t.Logf("As → scheme=%q wrapped=%v", ue.Scheme, err)
}
