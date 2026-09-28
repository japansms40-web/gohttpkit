package interceptor_test

import (
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestRequestMutator_mutate为nil不炸(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(interceptor.DefaultChain(), interceptor.NewRequestMutatorInterceptor(nil))
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("mutate=nil body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRequestMutator_非nil改写生效(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(interceptor.DefaultChain(),
			interceptor.NewRequestMutatorInterceptor(func(req *httpx.Request) {
				req.HTTPReq.Header.Set("x-mutated", "1")
			}))
	})
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	srv.mu <- struct{}{}
	t.Logf("服务器收到 x-mutated=%q", last.Header.Get("x-mutated"))
	if last.Header.Get("x-mutated") != "1" {
		t.Fatalf("请求改写器未生效，x-mutated=%q", last.Header.Get("x-mutated"))
	}
}
