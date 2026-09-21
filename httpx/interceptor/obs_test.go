package interceptor_test

// interceptors_obs_test.go —— 观察层拦截器的单元契约。

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

// ─────────────────────────── 可选拦截器的剩余分支 ───────────────────────────

func TestHTMLSaveInterceptor(t *testing.T) {
	t.Run("只保存 text/html", func(t *testing.T) {
		var saved [][]byte
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/html" {
				w.Header().Set("content-type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte("<p>hi</p>"))
				return
			}
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"a":1}`))
		})
		c := newClient(t, srv.Server, func(o *httpx.Options) {
			o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
				interceptor.NewHTMLSaveInterceptor(func(b []byte) { saved = append(saved, b) }))
		})
		if _, err := c.Get(context.Background(), "/html", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(context.Background(), "/json", nil); err != nil {
			t.Fatal(err)
		}
		first := ""
		if len(saved) > 0 {
			first = string(saved[0])
		}
		t.Logf("saved=%d first=%q", len(saved), first)
		if len(saved) != 1 || string(saved[0]) != "<p>hi</p>" {
			t.Fatalf("saved = %v", saved)
		}
	})

	t.Run("sink 为 nil 不炸", func(t *testing.T) {
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/html")
			_, _ = w.Write([]byte("<p>hi</p>"))
		})
		c := newClient(t, srv.Server, func(o *httpx.Options) {
			o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLSaveInterceptor(nil))
		})
		body, err := c.Get(context.Background(), "/x", nil)
		t.Logf("sink=nil body=%q err=%v", body, err)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("出错时不保存", func(t *testing.T) {
		var saved int
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: "https://x.example"},
			Interceptors: httpx.Interceptors{
				interceptor.NewHTMLSaveInterceptor(func([]byte) { saved++ }),
				httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
					return nil, errors.New("boom")
				}),
			},
		})
		_, err := c.Get(context.Background(), "/x", nil)
		t.Logf("inner err=%v saved=%d", err, saved)
		if err == nil {
			t.Fatal("want error")
		}
		if saved != 0 {
			t.Fatal("出错不该保存")
		}
	})
}

func TestTransactionInterceptor_sink为nil与出错不回调(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewTransactionInterceptor(nil))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("sink=nil ok")

	var called int
	c2 := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewTransactionInterceptor(func(*httpx.Transaction) { called++ }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, errors.New("boom") }),
		},
	})
	_, err := c2.Get(context.Background(), "/x", nil)
	t.Logf("resp=nil err=%v called=%d", err, called)
	if err == nil {
		t.Fatal("want error")
	}
	if called != 0 {
		t.Fatal("resp 为 nil 时不该回调")
	}
}

// ─────────────────────────── 观察层与核心层的剩余分支 ───────────────────────────

func TestLogging_慢请求降级为warn(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	})
	// 全量与摘要两种日志形态都走一遍 slow 分支
	for _, summary := range []bool{false, true} {
		c := newClient(t, srv.Server, func(o *httpx.Options) {
			o.LogSummaryOnly = summary
			o.SlowMS = time.Millisecond
		})
		if _, err := c.Get(context.Background(), "/x", nil); err != nil {
			t.Fatal(err)
		}
		t.Logf("summary=%v slowMS=%v", summary, time.Millisecond)
	}
}

func TestLogging_摘要模式下4xx仍打全量(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("nope"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.LogSummaryOnly = true })
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("summary 4xx status=%d", c.SnapshotResponseStatusCode())
	if c.SnapshotResponseStatusCode() != 404 {
		t.Fatal("状态码没记上")
	}
}

func TestLogging_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewLoggingInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("logging inner err=%v is=%v", err, errors.Is(err, sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusCodeCache与响应头缓存_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	for name, chain := range map[string]httpx.Interceptors{
		"statusCodeCache":     {interceptor.NewStatusCodeCacheInterceptor(), failing(sentinel)},
		"responseHeaderCache": {interceptor.NewResponseHeaderCacheInterceptor(), failing(sentinel)},
		"bodyDecode":          {interceptor.NewBodyDecodeInterceptor(), failing(sentinel)},
	} {
		t.Run(name, func(t *testing.T) {
			c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}, Interceptors: chain})
			_, err := c.Get(context.Background(), "/x", nil)
			t.Logf("%s err=%v is=%v", name, err, errors.Is(err, sentinel))
			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func failing(err error) httpx.Interceptor {
	return httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, err })
}
