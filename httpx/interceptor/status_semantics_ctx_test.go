package interceptor_test

// status_semantics_ctx_test.go —— NewStatusSemanticsInterceptorContext 的单元契约：
// 规则拿到本次请求的 ctx、2xx 不进规则、nil 规则回落 DefaultStatusRule、规则放行与内层错误穿透。

import (
	"context"
	"errors"
	"net/http"
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

type ctxKey struct{}

func TestStatusSemanticsContext_规则拿到请求ctx(t *testing.T) {
	sentinel := errors.New("rule")
	var gotValue any
	var gotStatus int
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewStatusSemanticsInterceptorContext(func(ctx context.Context, status int, _ []byte) error {
				gotValue, gotStatus = ctx.Value(ctxKey{}), status
				return sentinel
			}))
	})
	ctx := context.WithValue(context.Background(), ctxKey{}, "trace-me")
	_, err := c.Get(ctx, "/x", nil)
	t.Logf("err=%v ctxValue=%v status=%d", err, gotValue, gotStatus)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 规则返回的错误", err)
	}
	if gotValue != "trace-me" {
		t.Fatalf("规则拿到的 ctx 值 = %v, want 调用方 ctx 的值", gotValue)
	}
	if gotStatus != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", gotStatus)
	}
}

func TestStatusSemanticsContext_2xx不进规则(t *testing.T) {
	var ruleCalls int
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewStatusSemanticsInterceptorContext(func(context.Context, int, []byte) error { ruleCalls++; return errors.New("x") }))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("2xx body=%q err=%v ruleCalls=%d", body, err, ruleCalls)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body=%q err=%v, want (ok, nil)", body, err)
	}
	if ruleCalls != 0 {
		t.Fatal("2xx 不该进规则")
	}
}

func TestStatusSemanticsContext_nil规则回落默认规则(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewStatusSemanticsInterceptorContext(nil))
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("nil rule err=%v", err)
	var se *kiterrors.HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want *HTTPStatusError{404}（DefaultStatusRule 空体语义）", err)
	}
}

func TestStatusSemanticsContext_规则放行时原样返回响应(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"msg":"biz"}`))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewStatusSemanticsInterceptorContext(func(context.Context, int, []byte) error { return nil }))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("pass body=%q err=%v status=%d", body, err, c.SnapshotResponseStatusCode())
	if err != nil || string(body) != `{"msg":"biz"}` {
		t.Fatalf("body=%q err=%v, want 原样放行", body, err)
	}
	if c.SnapshotResponseStatusCode() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", c.SnapshotResponseStatusCode())
	}
}

func TestStatusSemanticsContext_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	var ruleCalls int
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewStatusSemanticsInterceptorContext(func(context.Context, int, []byte) error { ruleCalls++; return nil }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("inner err=%v ruleCalls=%d", err, ruleCalls)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 内层错误原样穿透", err)
	}
	if ruleCalls != 0 {
		t.Fatal("内层已失败时不该进规则")
	}
}
