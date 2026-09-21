package interceptor_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestNewClient_未传链时装默认链(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c, err := interceptor.NewClient(httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("len=%d", len(c.Interceptors()))
	if got := len(c.Interceptors()); got != len(interceptor.DefaultChain()) {
		t.Fatalf("len = %d, want %d", got, len(interceptor.DefaultChain()))
	}
	body, err := c.Get(context.Background(), "/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestNewClient_空切片不回落默认链(t *testing.T) {
	c, err := interceptor.NewClient(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/x", nil)
	var ce *httpx.ChainExhaustedError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *ChainExhaustedError", err, err)
	}
	t.Logf("Index=%d Length=%d", ce.Index, ce.Length)
}
