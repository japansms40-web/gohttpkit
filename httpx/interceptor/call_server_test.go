package interceptor_test

// call_server_test.go —— NewCallServerInterceptor 的单元契约：是终端、不调内层、用共享 HTTPClient 跟随重定向。
// 网络错误包装与响应字段映射是两种终端共用的 doHTTP 契约，见 do_http_test.go。

import (
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestCallServer_是终端且不调用内层(t *testing.T) {
	if !httpx.IsTerminal(interceptor.NewCallServerInterceptor()) {
		t.Fatal("callServer 必须内嵌 TerminalMarker，链编辑才能找到插入点")
	}
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var innerCalls int
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Interceptors{
			interceptor.NewBridgeInterceptor(),
			interceptor.NewBodyDecodeInterceptor(),
			interceptor.NewCallServerInterceptor(),
			countingInterceptor(&innerCalls),
		}
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("body=%q err=%v server=%d inner=%d", body, err, srv.count(), innerCalls)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if srv.count() != 1 || innerCalls != 0 {
		t.Fatalf("应发 1 次请求且不调内层，server=%d inner=%d", srv.count(), innerCalls)
	}
}

func TestCallServer_跟随重定向到最终响应(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/from" {
			http.Redirect(w, r, "/to", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.Interceptors = interceptor.DefaultChain() })
	body, err := c.Get(t.Context(), "/from", nil)
	t.Logf("302 → body=%q err=%v status=%d server=%d", body, err, c.SnapshotResponseStatusCode(), srv.count())
	if err != nil || string(body) != "final" {
		t.Fatalf("默认终端应跟随 302 拿到最终体，body=%q err=%v", body, err)
	}
	if c.SnapshotResponseStatusCode() != http.StatusOK || srv.count() != 2 {
		t.Fatalf("status=%d server=%d，want 200 / 2", c.SnapshotResponseStatusCode(), srv.count())
	}
}
