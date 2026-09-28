package httpx_test

// client_test.go —— Client 构造访问器与 Do 入口的单元契约。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/netproxy"
)

// ─────────────────────────────── client.go ───────────────────────────────

func TestPostJSON_自动带contentType(t *testing.T) {
	var gotCT, gotBody string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.PostJSON(t.Context(), "/x", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	gotBody = string(srv.bodies[len(srv.bodies)-1])
	srv.mu <- struct{}{}
	t.Logf("content-type=%q body=%q", gotCT, gotBody)
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if gotBody != `{"n":1}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestClient_访问器(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	hp := httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"accept": "*/*"}}
	c, err := httpx.NewClient(httpx.Options{Headers: hp, ProxyURL: "", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("BaseURL=%q interceptors=%d timeout=%v retries=%d", c.Headers().BaseURL(), len(c.Interceptors()), c.HTTPClient.Timeout, c.RetryPolicy().MaxRetries)
	if c.Headers().BaseURL() != srv.URL {
		t.Fatal("Headers() 不对")
	}
	if got := len(c.Interceptors()); got != len(interceptor.DefaultChain()) {
		t.Fatalf("Interceptors() 长度 = %d", got)
	}
	// 返回的是副本：改它不该影响客户端
	chain := c.Interceptors()
	chain[0] = nil
	if c.Interceptors()[0] == nil {
		t.Fatal("Interceptors() 返回了内部切片")
	}
	if c.Options().Timeout != 5*time.Second {
		t.Fatal("Options() 不对")
	}
	if c.RetryPolicy().MaxRetries != 3 {
		t.Fatalf("默认重试次数 = %d, want 3", c.RetryPolicy().MaxRetries)
	}
	if c.HTTPClient.Timeout != 5*time.Second {
		t.Fatalf("Timeout 未生效: %v", c.HTTPClient.Timeout)
	}
	c.SetTimeout(time.Second)
	t.Logf("SetTimeout → %v", c.HTTPClient.Timeout)
	if c.HTTPClient.Timeout != time.Second {
		t.Fatal("SetTimeout 未生效")
	}
}

func TestClient_SlowMS固化阈值(t *testing.T) {
	var nilClient *httpx.Client
	t.Logf("nil SlowMS=%d", nilClient.SlowMS())
	if got := nilClient.SlowMS(); got != 0 {
		t.Fatalf("nil client SlowMS = %d, want 0", got)
	}

	c, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		SlowMS:  1500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SlowMS=%d", c.SlowMS())
	if got := c.SlowMS(); got != 1500 {
		t.Fatalf("SlowMS = %d, want 1500", got)
	}
}

func TestClient_CacheStatusCode与响应头(t *testing.T) {
	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
	if err != nil {
		t.Fatal(err)
	}
	c.CacheStatusCode(404)
	h := make(http.Header)
	h.Set("content-type", "text/html")
	c.CacheResponseHeaders(h)
	t.Logf("status=%d ct=%q", c.SnapshotResponseStatusCode(), c.SnapshotResponseHeaders().Get("content-type"))
	if got := c.SnapshotResponseStatusCode(); got != 404 {
		t.Fatalf("status = %d, want 404", got)
	}
	if got := c.SnapshotResponseHeaders().Get("content-type"); got != "text/html" {
		t.Fatalf("header = %q", got)
	}
}

func TestLogBodyLimit_三种情形(t *testing.T) {
	var nilClient *httpx.Client
	t.Logf("nil=%d default=%d truncatedOff=%d", nilClient.LogBodyLimit(), httpx.LogBodyMaxBytes, 0)
	if got := nilClient.LogBodyLimit(); got != httpx.LogBodyMaxBytes {
		t.Fatalf("nil client = %d, want 回落默认(截断开启)", got)
	}

	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("default client limit=%d", c.LogBodyLimit())
	if got := c.LogBodyLimit(); got != httpx.LogBodyMaxBytes {
		t.Fatalf("默认 = %d", got)
	}

	c2, err := httpx.New(httpx.Options{
		Headers:                  httpx.StaticHeaders{Base: "https://x.example"},
		DisableLogBodyTruncation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("DisableLogBodyTruncation limit=%d", c2.LogBodyLimit())
	if got := c2.LogBodyLimit(); got != 0 {
		t.Fatalf("关闭截断时 = %d, want 0", got)
	}
}

func TestDo_body编码失败时不发请求(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(t.Context(), httpx.RequestSpec{Method: http.MethodPost, Path: "/x", Body: make(chan int)})
	t.Logf("encode fail err=%v (%T) served=%d", err, err, srv.count())
	if err == nil {
		t.Fatal("want error")
	}
	if srv.count() != 0 {
		t.Fatal("编码失败不该发出网络请求")
	}
}

func TestDo_空Method默认GET(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	if _, err := c.Do(t.Context(), httpx.RequestSpec{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	t.Logf("empty method → %q", srv.last().Method)
	if got := srv.last().Method; got != http.MethodGet {
		t.Fatalf("method = %q", got)
	}
}

func TestDo_空链不再回落默认链(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c, err := httpx.New(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Interceptors: httpx.Interceptors{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(t.Context(), "/x", nil)
	t.Logf("empty chain err=%v", err)
	assertChainExhausted(t, err, 0, 0)
}

func TestNew_缺HeaderProvider报错(t *testing.T) {
	_, err := httpx.New(httpx.Options{})
	assertMissingHeaderProvider(t, err, "Options.Headers")
}

func TestNew_非法代理URL仍可As到netproxy类型(t *testing.T) {
	_, err := httpx.New(httpx.Options{
		Headers:  httpx.StaticHeaders{Base: "https://x.example"},
		ProxyURL: "ftp://1.2.3.4:1080",
	})
	if err == nil {
		t.Fatal("不支持的代理 scheme 应在构造期报错")
	}
	if !strings.HasPrefix(err.Error(), "httpx.build_transport:") {
		t.Fatalf("err = %v, want 保留 httpx.build_transport 前缀", err)
	}
	var outer *kiterrors.Error
	if !errors.As(err, &outer) || outer.Op != "httpx.build_transport" {
		t.Fatalf("外层应为结构化错误并保留步骤，got %T %v", err, err)
	}
	var ue *netproxy.UnsupportedProxySchemeError
	if !errors.As(err, &ue) || ue.Scheme != "ftp" {
		t.Fatalf("err = %v (%T), want *netproxy.UnsupportedProxySchemeError Scheme=ftp", err, err)
	}
	t.Logf("As → scheme=%q wrapped=%v", ue.Scheme, err)
}

func TestWithChain_nil只保留最外层旁路(t *testing.T) {
	parent, err := httpx.NewClient(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://a.example"},
		Interceptors: httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	child := parent.WithChain(nil)
	got := child.Interceptors()
	t.Logf("WithChain(nil) len=%d", len(got))
	if len(got) != 1 {
		t.Fatalf("默认链最外层只有 transaction 是 SideChannel，len=%d", len(got))
	}
	if !httpx.IsSideChannel(got[0]) {
		t.Fatal("继承层应是 SideChannel")
	}
}

func TestSlowMS_亚毫秒截断为0(t *testing.T) {
	c, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://a.example"},
		SlowMS:  500 * time.Microsecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("500µs → SlowMS()=%d", c.SlowMS())
	if c.SlowMS() != 0 {
		t.Fatalf("不足 1ms 应截断为 0（关闭），got %d", c.SlowMS())
	}
}

func TestCacheResponseHeaders_nil清空(t *testing.T) {
	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://a.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.SnapshotResponseHeaders() != nil || c.SnapshotResponseStatusCode() != 0 {
		t.Fatal("New 后快照应为零值")
	}
	c.CacheResponseHeaders(map[string][]string{"x": {"1"}})
	c.CacheResponseHeaders(nil)
	t.Logf("清空后 Snapshot=%v", c.SnapshotResponseHeaders())
	if c.SnapshotResponseHeaders() != nil {
		t.Fatal("CacheResponseHeaders(nil) 应清空")
	}
}

// —— 并发回归。
// 单个 *Client 会被多 goroutine 共享（几十上百路并发是常态），
// 「最近一次响应」缓存与 HeaderProvider 的读写必须扛得住。
// 在装了 C 编译器的机器上用 `make race` 跑，才能真正发挥这些用例的价值。

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
	c, err := httpx.NewClient(httpx.Options{
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
				if _, err := c.Get(t.Context(), fmt.Sprintf("/w%d/%d", w, i), nil); err != nil {
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
	got := served.Load()
	t.Logf("workers=%d perWorker=%d served=%d", workers, perWorker, got)
	if got != workers*perWorker {
		t.Fatalf("服务端收到 %d 次，want %d", got, workers*perWorker)
	}
}

func TestConcurrent_派生子Client与父并发互不干扰(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := httpx.NewClient(httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	derived := c.WithChain(interceptor.NoRedirectChain())

	const pairs = 16
	t.Logf("parent+derived pairs=%d", pairs)
	var wg sync.WaitGroup
	for i := 0; i < pairs; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = c.Get(t.Context(), "/p", nil) }()
		go func() { defer wg.Done(); _, _ = derived.Get(t.Context(), "/d", nil) }()
	}
	wg.Wait()
}
