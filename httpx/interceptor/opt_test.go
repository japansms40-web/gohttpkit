package interceptor_test

// interceptors_opt_test.go —— 可选拦截器的单元契约。

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestClassifyInterceptor_classify为nil时放行(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewClassifyInterceptor(nil))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("classify=nil body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassifyInterceptor_内层出错时不覆盖结论(t *testing.T) {
	sentinel := errors.New("inner")
	var classified int
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewClassifyInterceptor(func(int, []byte) error { classified++; return errors.New("outer") }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("inner err=%v classified=%d", err, classified)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 内层错误原样穿透", err)
	}
	if classified != 0 {
		t.Fatal("内层已判失败时不该再跑归类")
	}
}

func TestStatusSemantics_2xx直接放行(t *testing.T) {
	var ruleCalls int
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewStatusSemanticsInterceptor(func(int, []byte) error { ruleCalls++; return errors.New("x") }))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("2xx body=%q err=%v ruleCalls=%d", body, err, ruleCalls)
	if err != nil {
		t.Fatal(err)
	}
	if ruleCalls != 0 {
		t.Fatal("2xx 不该进规则")
	}
}

func TestStatusSemantics_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewStatusSemanticsInterceptor(nil),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("status inner err=%v is=%v", err, errors.Is(err, sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestHTMLText_非HTML响应不动(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"html":"<p>x</p>"}`))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("<p>"))
	})
	body, err := c.Get(context.Background(), "/x", nil)
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
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("htmlText inner err=%v is=%v", err, errors.Is(err, sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestMutator_mutate为nil不炸(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(interceptor.DefaultChain(), interceptor.NewRequestMutatorInterceptor(nil))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("mutate=nil body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
}
