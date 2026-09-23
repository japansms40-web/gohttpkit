package httpx_test

// helpers_test.go —— 跨测试文件共用的客户端与假服务器脚手架。
// TestMain 留在 characterization_test.go。

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
	_ "github.com/japansms40-web/gohttpkit/httpx/interceptor" // 注册 httpx.NewClient 的默认链
)

// newClient 建一个指向 srv 的客户端，headers 为构头结果（nil 用一组最小头）。
func newClient(t *testing.T, srv *httptest.Server, mutate func(*httpx.Options)) *httpx.Client {
	t.Helper()
	opts := httpx.Options{
		Headers: httpx.StaticHeaders{
			Base:    srv.URL,
			Headers: map[string]string{"accept": "application/json", "user-agent": "kit-test/1.0"},
		},
		// 测试里把退避压到 1ms，避免每个用例都真等 200/400/800ms。
		// 退避【时序】本身另有专门用例单独锁定。
		Retry: httpx.WithRetry(3, time.Millisecond, 0),
	}
	if mutate != nil {
		mutate(&opts)
	}
	c, err := httpx.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// recordingServer 记录收到的请求，供断言请求侧行为。
type recordingServer struct {
	*httptest.Server
	mu       chan struct{}
	requests []*http.Request
	bodies   [][]byte
}

func newRecordingServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *recordingServer {
	t.Helper()
	rs := &recordingServer{mu: make(chan struct{}, 1)}
	rs.mu <- struct{}{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		<-rs.mu
		rs.requests = append(rs.requests, r.Clone(context.Background()))
		rs.bodies = append(rs.bodies, body)
		rs.mu <- struct{}{}
		handler(w, r)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) count() int {
	<-rs.mu
	n := len(rs.requests)
	rs.mu <- struct{}{}
	return n
}

func (rs *recordingServer) last() *http.Request {
	<-rs.mu
	defer func() { rs.mu <- struct{}{} }()
	if len(rs.requests) == 0 {
		return nil
	}
	return rs.requests[len(rs.requests)-1]
}

// newClientWith 用给定 Options 建客户端（失败即 fatal）。
func newClientWith(t *testing.T, opts httpx.Options) *httpx.Client {
	t.Helper()
	c, err := httpx.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}
