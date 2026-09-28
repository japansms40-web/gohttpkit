package interceptor_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestBridge_创建请求失败(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(t.Context(), httpx.RequestSpec{Method: "BAD METHOD", Path: "/x"})
	var cre *httpx.CreateHTTPRequestError
	if !errors.As(err, &cre) || cre.Method != "BAD METHOD" || cre.Err == nil {
		t.Fatalf("err = %v (%T), want *CreateHTTPRequestError Method=BAD METHOD", err, err)
	}
	t.Logf("CreateHTTPRequestError Method=%q unwrap=%v", cre.Method, cre.Err)
	if srv.count() != 0 {
		t.Fatal("请求都没建出来，不该发出去")
	}
}

type nilHeaders struct{ base string }

func (n nilHeaders) BuildHeaders(context.Context) map[string]string { return nil }

func (n nilHeaders) BaseURL() string { return n.base }

// 与 httpx/characterization_test.go 的 TestBuildHeaders返回nil时请求不发出 重复是有意的：
// 本条是 bridge 拦截器的单元测试，那条是整个 Client 的行为锚点。
//
//goland:noinspection DuplicatedCode
func TestBridge_构头返回nil不发请求(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.HeaderProviderFunc{Base: srv.URL, Build: func(context.Context) map[string]string { return nil }}
	})
	_, err := c.Get(t.Context(), "/x", nil)
	assertNilBuildHeaders(t, err, "httpx.HeaderProviderFunc")
	if srv.count() != 0 {
		t.Fatal("构头失败时不应发出网络请求")
	}
}

func TestBridge_自定义Provider构头nil带类型名(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = nilHeaders{base: srv.URL}
	})
	_, err := c.Get(t.Context(), "/x", nil)
	assertNilBuildHeaders(t, err, "interceptor_test.nilHeaders")
	if srv.count() != 0 {
		t.Fatal("构头失败时不应发出网络请求")
	}
}

func TestBridge_POST体与白名单与空ExtraHeaders(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.DefaultChain()
	})
	_, err := c.Do(t.Context(), httpx.RequestSpec{
		Method:          http.MethodPost,
		Path:            "/x",
		Body:            []byte("hello!!"),
		HeaderWhitelist: map[string]string{"accept": "", "content-length": "7"},
		ExtraHeaders:    map[string]string{"x-skip": "", "x-keep": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	gotBody := srv.bodies[len(srv.bodies)-1]
	srv.mu <- struct{}{}
	t.Logf("method=%s body=%q cl=%d ua=%q accept=%q skip=%q keep=%q origin=%q",
		last.Method, gotBody, last.ContentLength, last.Header.Get("user-agent"), last.Header.Get("accept"),
		last.Header.Get("x-skip"), last.Header.Get("x-keep"), last.Header.Get("origin"))
	if last.Method != http.MethodPost || string(gotBody) != "hello!!" {
		t.Fatalf("POST 体没发出: method=%s body=%q", last.Method, gotBody)
	}
	if last.ContentLength != 7 {
		t.Fatalf("合法 content-length 应写入 Request.ContentLength，got %d", last.ContentLength)
	}
	if ua := last.Header.Get("User-Agent"); ua != "" && ua != "kit-test/1.0" {
		// 严格模式应抑制 Go 默认 UA（Go-http-client/1.1）
		if strings.Contains(ua, "Go-http-client") {
			t.Fatalf("严格白名单泄漏了 Go 默认 UA: %q", ua)
		}
	}
	if last.Header.Get("x-skip") != "" {
		t.Fatal("ExtraHeaders 空值应跳过")
	}
	if last.Header.Get("x-keep") != "1" {
		t.Fatalf("x-keep=%q", last.Header.Get("x-keep"))
	}
}

func TestBridge_DisableOriginReferer不注入(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.DisableOriginReferer = true
	})
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	srv.mu <- struct{}{}
	t.Logf("origin=%q referer=%q", last.Header.Get("origin"), last.Header.Get("referer"))
	if last.Header.Get("origin") != "" || last.Header.Get("referer") != "" {
		t.Fatal("DisableOriginReferer 后不应注入 origin/referer")
	}
}

func TestApplySpecialHeaders_host头消化(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.Interceptors = interceptor.DefaultChain() })
	_, err := c.Do(t.Context(), httpx.RequestSpec{
		Path:         "/x",
		ExtraHeaders: map[string]string{"host": "custom.example.test", "content-length": "not-a-number"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 取服务器实际收到的请求，Host 应被 applySpecialHeaders 写进 Request.Host。
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	srv.mu <- struct{}{}
	t.Logf("服务器收到 Host=%q（非法 content-length 已被静默丢弃、不崩）", last.Host)
	if last.Host != "custom.example.test" {
		t.Fatalf("Host=%q，应被 host 头覆盖为 custom.example.test", last.Host)
	}
}
