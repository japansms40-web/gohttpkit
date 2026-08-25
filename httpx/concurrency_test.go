package httpx_test

// concurrency_test.go —— 并发回归。
// 单个 *Client 会被多 goroutine 共享（几十上百路并发是常态），
// 「最近一次响应」缓存与 HeaderProvider 的读写必须扛得住。
// 在装了 C 编译器的机器上用 `make race` 跑，才能真正发挥这些用例的价值。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// statefulHeaders 模拟带会话状态的构头器：token 会被响应回写改写，
// 同时被并发的构头读取 —— 这正是最容易出 race 的形态。
type statefulHeaders struct {
	base  string
	mu    sync.RWMutex
	token string
}

func (h *statefulHeaders) BuildHeaders(context.Context) map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]string{"accept": "application/json", "x-token": h.token}
}
func (h *statefulHeaders) BaseURL() string { return h.base }
func (h *statefulHeaders) setToken(v string) {
	h.mu.Lock()
	h.token = v
	h.mu.Unlock()
}

func TestConcurrent_共享Client并发请求(t *testing.T) {
	var served atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := served.Add(1)
		w.Header().Set("x-new-token", fmt.Sprintf("t-%d", n))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	hp := &statefulHeaders{base: srv.URL, token: "t-0"}
	c, err := httpx.New(httpx.Options{
		Headers: hp,
		// 响应回写：把服务端下发的新 token 写回构头器，下一次请求带上。
		OnResponseHeaders: func(_ context.Context, h http.Header) {
			if v := h.Get("x-new-token"); v != "" {
				hp.setToken(v)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	const workers, perWorker = 32, 8
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if _, err := c.Get(context.Background(), fmt.Sprintf("/w%d/%d", w, i), nil); err != nil {
					errs <- err
					return
				}
				// 并发读快照：不加锁直读字段会被 race detector 抓到
				_ = c.SnapshotResponseStatusCode()
				_ = c.SnapshotResponseHeaders()
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发请求失败: %v", err)
	}
	if got := served.Load(); got != workers*perWorker {
		t.Fatalf("服务端收到 %d 次，want %d", got, workers*perWorker)
	}
}

func TestConcurrent_派生子Client与父并发互不干扰(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	derived := c.WithChain(httpx.NoRedirectChain())

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = c.Get(context.Background(), "/p", nil) }()
		go func() { defer wg.Done(); _, _ = derived.Get(context.Background(), "/d", nil) }()
	}
	wg.Wait()
}
