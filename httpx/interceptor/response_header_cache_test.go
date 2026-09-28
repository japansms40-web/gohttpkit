package interceptor_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestResponseHeaderCache_多值SetCookie进hook(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("set-cookie", "a=1")
		w.Header().Add("set-cookie", "b=2")
		_, _ = w.Write([]byte("ok"))
	})
	var cookies []string
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.OnResponseHeaders = func(_ context.Context, h http.Header) {
			cookies = append([]string(nil), h.Values("set-cookie")...)
		}
	})
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("hook cookies=%v snapshot=%v", cookies, c.SnapshotResponseHeaders().Values("set-cookie"))
	if len(cookies) != 2 {
		t.Fatalf("hook 应收到两条 Set-Cookie，got %v", cookies)
	}
}

func TestResponseHeaderCache_OnResponseHeaders回调(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-new-token", "abc123")
		_, _ = w.Write([]byte("ok"))
	})
	var gotToken string
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.DefaultChain()
		o.OnResponseHeaders = func(_ context.Context, h http.Header) { gotToken = h.Get("x-new-token") }
	})
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("hook 收到 x-new-token=%q", gotToken)
	if gotToken != "abc123" {
		t.Fatalf("hook 未被调用或未拿到头，gotToken=%q", gotToken)
	}
}
