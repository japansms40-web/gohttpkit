package interceptor_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestNoRedirectTerminal_传输错误也包成TransportError(t *testing.T) {
	// 禁重定向终端和默认终端必须对错误做同样的包装，否则那条链上的重试会失效。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	var attempts int
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Retry:        httpx.WithRetry(2, time.Millisecond, 0),
		Interceptors: httpx.SpliceBeforeTerminal(interceptor.NoRedirectChain(), countingInterceptor(&attempts)),
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("noRedirect transport err=%v attempts=%d", err, attempts)
	if err == nil {
		t.Fatal("want error")
	}
	if attempts != 3 {
		t.Fatalf("尝试 %d 次，want 3(禁重定向终端的错误同样应触发重试)", attempts)
	}
}

func countingInterceptor(n *int) httpx.Interceptor {
	return httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		*n++
		return ch.Proceed()
	})
}

func TestNoRedirect_302原样返回不跟随(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/from" {
			w.Header().Set("location", "/to")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("followed"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.NoRedirectChain()
	})
	body, err := c.Get(t.Context(), "/from", nil)
	t.Logf("302 body=%q status=%d location=%q hits=%d", body, c.SnapshotResponseStatusCode(), c.SnapshotResponseHeaders().Get("location"), srv.count())
	if err != nil {
		t.Fatalf("302 空体不该报错: %v", err)
	}
	if string(body) != "" {
		t.Fatalf("body=%q，跟随了重定向", body)
	}
	if got := c.SnapshotResponseStatusCode(); got != http.StatusFound {
		t.Fatalf("status=%d, want 302", got)
	}
	if got := c.SnapshotResponseHeaders().Get("location"); got != "/to" {
		t.Fatalf("location=%q", got)
	}
	if srv.count() != 1 {
		t.Fatalf("应只打 /from 一次，hits=%d", srv.count())
	}
}
