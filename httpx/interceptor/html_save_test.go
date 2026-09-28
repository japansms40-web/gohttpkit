package interceptor_test

import (
	"errors"
	"net/http"
	"testing"

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
		if _, err := c.Get(t.Context(), "/html", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(t.Context(), "/json", nil); err != nil {
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
		body, err := c.Get(t.Context(), "/x", nil)
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
		_, err := c.Get(t.Context(), "/x", nil)
		t.Logf("inner err=%v saved=%d", err, saved)
		if err == nil {
			t.Fatal("want error")
		}
		if saved != 0 {
			t.Fatal("出错不该保存")
		}
	})
}
