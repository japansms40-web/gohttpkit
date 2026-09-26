package interceptor_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

// 本文件锁定 interceptor 包 init() 把 DefaultChain 注册进 httpx：import 本包后 httpx.NewClient 开箱即用。

func TestHTTPXNewClient_未传链时装默认链(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c, err := httpx.NewClient(httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("len=%d", len(c.Interceptors()))
	if got := len(c.Interceptors()); got != len(interceptor.DefaultChain()) {
		t.Fatalf("len = %d, want %d", got, len(interceptor.DefaultChain()))
	}
	body, err := c.Get(t.Context(), "/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestHTTPXNewClient_空切片不回落默认链(t *testing.T) {
	c, err := httpx.NewClient(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(t.Context(), "/x", nil)
	var ce *httpx.ChainExhaustedError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *ChainExhaustedError", err, err)
	}
	t.Logf("Index=%d Length=%d", ce.Index, ce.Length)
}
