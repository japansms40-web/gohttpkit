package interceptor_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	liberrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestHTMLText_非HTML响应不动(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"html":"<p>x</p>"}`))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("<p>"))
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("non-html body=%q err=%v", body, err)
	if err != nil {
		t.Fatalf("非 HTML 响应不该被错误页标记误伤: %v", err)
	}
	if string(body) != `{"html":"<p>x</p>"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestHTMLText_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewHTMLTextInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("htmlText inner err=%v is=%v", err, errors.Is(err, sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestHTMLText_空marker忽略不误伤(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte("<html><body>Access Denied</body></html>"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("", ""))
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("空 marker body=%q err=%v", body, err)
	if err != nil {
		t.Fatalf("空 marker 不应把正文当错误页: %v", err)
	}
	if !strings.Contains(string(body), "Access Denied") {
		t.Fatalf("应留下可见文案，got %q", body)
	}
}

func TestHTMLText_提纯剥script(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><script>var t='secret'</script><p>你好 世界</p></body></html>"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor())
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("HTML 提纯 → body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if got != "你好 世界" {
		t.Fatalf("提纯结果=%q，应剥掉 script、只留可见文案", got)
	}
}

func TestHTMLText_命中错误页标记报错(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte("<html><body>Access Denied</body></html>"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("", "Access Denied"))
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("HTML 命中标记 → err=%v", err)
	if _, ok := errors.AsType[*liberrors.HTTPStatusError](err); !ok {
		t.Fatalf("err=%v，应为命中标记的 *HTTPStatusError", err)
	}
}
