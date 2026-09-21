package httpx_test

// characterization_test.go —— 行为锁定套件。
//
// 这些测试断言的不是「代码怎么写的」，而是「客户端对外表现出什么行为」：
// 重试几次、退避多久、哪些错误不重试、白名单怎么过滤、头的大小写、
// 状态码与响应头在什么时机被缓存、链的执行顺序。
//
// 它们是本库唯一的安全网 —— 拦截器链的顺序一旦被改动，破坏的往往不是编译，
// 而是某个只在生产环境偶发的行为。改链之前先跑 `make char`。

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/logger"
	"github.com/japansms40-web/gohttpkit/netproxy"
)

func TestMain(m *testing.M) {
	logger.SetConfig(logger.Config{
		Level:  logger.LevelError,
		Format: logger.FormatJSON,
		Output: logger.OutputConsole,
	})
	os.Exit(m.Run())
}

// 共享脚手架见 helpers_test.go。

// ─────────────────────────────── 基础行为 ───────────────────────────────

func TestDoRequest_成功返回响应体(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	c := newClient(t, srv.Server, nil)

	body, err := c.Get(context.Background(), "/ping", nil)
	t.Logf("body=%q err=%v status=%d", body, err, c.SnapshotResponseStatusCode())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
	if got := c.SnapshotResponseStatusCode(); got != 200 {
		t.Fatalf("status = %d, want 200", got)
	}
}

func TestDoRequest_非2xx不报错且体原样返回(t *testing.T) {
	// 锁定核心取舍：基建层不替调用方对状态码下结论。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"bad_request"}`))
	})
	c := newClient(t, srv.Server, nil)

	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("body=%q err=%v status=%d", body, err, c.SnapshotResponseStatusCode())
	if err != nil {
		t.Fatalf("默认链不该把非 2xx 变成 error, got %v", err)
	}
	if string(body) != `{"error":"bad_request"}` {
		t.Fatalf("body = %q", body)
	}
	if got := c.SnapshotResponseStatusCode(); got != 400 {
		t.Fatalf("status = %d, want 400", got)
	}
}

func TestDoRequest_查询参数与路径拼接(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)

	_, err := c.Get(context.Background(), "/search", url.Values{"q": {"hello world"}, "n": {"2"}})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got := srv.last().URL.String()
	t.Logf("url=%q", got)
	if got != "/search?n=2&q=hello+world" {
		t.Fatalf("url = %q", got)
	}
}

func TestDoRequest_绝对URL跨域直发且originReferer仍指向BaseURL(t *testing.T) {
	other := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("cross")) })
	main := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("main")) })
	c := newClient(t, main.Server, nil)

	body, err := c.Do(context.Background(), httpx.RequestSpec{
		Path:            other.URL + "/cross",
		HeaderWhitelist: map[string]string{"origin": "", "referer": ""},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Logf("body=%q origin=%q", body, other.last().Header.Get("origin"))
	if string(body) != "cross" {
		t.Fatalf("body = %q，绝对 URL 应直发到另一台服务器", body)
	}
	got := other.last()
	if got.Header.Get("origin") != main.URL {
		t.Fatalf("origin = %q, want %q(发起站点)", got.Header.Get("origin"), main.URL)
	}
}

// ─────────────────────────────── 请求头行为 ───────────────────────────────

func TestHeaders_nil白名单发送全量头(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)

	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	h := srv.last().Header
	t.Logf("accept=%q ua=%q origin=%q referer=%q", h.Get("accept"), h.Get("user-agent"), h.Get("origin"), h.Get("referer"))
	if h.Get("accept") != "application/json" || h.Get("user-agent") != "kit-test/1.0" {
		t.Fatalf("nil 白名单应发全量构建头, got %v", h)
	}
	if h.Get("origin") == "" || h.Get("referer") == "" {
		t.Fatalf("默认应注入 origin/referer, got %v", h)
	}
}

func TestHeaders_空白名单一个都不发(t *testing.T) {
	// 与 nil 的区别是本库最容易被误用的一处语义，必须锁死。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)

	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Path:            "/x",
		HeaderWhitelist: map[string]string{}, // 非 nil 的空 map
	})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.last().Header
	t.Logf("empty whitelist accept=%q origin=%q ua=%q", h.Get("accept"), h.Get("origin"), h.Get("user-agent"))
	if h.Get("accept") != "" || h.Get("origin") != "" || h.Get("user-agent") != "" {
		t.Fatalf("空白名单应一个构建头都不发, got %v", h)
	}
}

func TestHeaders_白名单空值取构建值非空值覆盖(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)

	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Path: "/x",
		HeaderWhitelist: map[string]string{
			"accept":     "",             // 取构建值
			"user-agent": "override/9.9", // 用白名单固定值
			"x-absent":   "",             // 构建值里没有 → 整条跳过
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.last().Header
	t.Logf("accept=%q ua=%q", h.Get("accept"), h.Get("user-agent"))
	if h.Get("accept") != "application/json" {
		t.Fatalf("accept = %q, want 构建值", h.Get("accept"))
	}
	if h.Get("user-agent") != "override/9.9" {
		t.Fatalf("user-agent = %q, want 白名单固定值", h.Get("user-agent"))
	}
	if _, ok := h["X-Absent"]; ok {
		t.Fatalf("构建值缺失的头不应被发出(哪怕空值)")
	}
}

func TestHeaders_extraHeaders覆盖构建值且空值跳过(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)

	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Path: "/x",
		ExtraHeaders: map[string]string{
			"user-agent": "extra/1.0",
			"accept":     "", // 空值跳过 → 保留构建值，而不是删掉 accept
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.last().Header
	t.Logf("ua=%q accept=%q", h.Get("user-agent"), h.Get("accept"))
	if h.Get("user-agent") != "extra/1.0" {
		t.Fatalf("user-agent = %q, want extra 覆盖", h.Get("user-agent"))
	}
	if h.Get("accept") != "application/json" {
		t.Fatalf("extra 的空值应跳过而非删头, accept = %q", h.Get("accept"))
	}
}

func TestHeaders_保持全小写不被规范化(t *testing.T) {
	// 这条是指纹保真的地基：标准库默认会把 sec-ch-ua 变成 Sec-Ch-Ua。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"sec-ch-ua": `"Chromium";v="140"`}}
	})

	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	// httptest 服务端读到的 Header 已被 net/http 规范化，故直接断言客户端侧写入的 map。
	// 这里改为经拦截器窥探实际写进 http.Request 的 key。
	var seen []string
	spy := interceptor.NewRequestMutatorInterceptor(func(r *httpx.Request) {
		for k := range r.HTTPReq.Header {
			seen = append(seen, k)
		}
	})
	c2, err := httpx.New(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"sec-ch-ua": `"Chromium";v="140"`}},
		Interceptors: httpx.SpliceBeforeTerminal(interceptor.DefaultChain(), spy),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c2.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("seen keys=%v", seen)
	for _, k := range seen {
		if k == "Sec-Ch-Ua" {
			t.Fatalf("header key 被规范化成了 %q，破坏指纹保真", k)
		}
	}
	if !contains(seen, "sec-ch-ua") {
		t.Fatalf("未见小写 sec-ch-ua, seen=%v", seen)
	}
}

func TestHeaders_白名单声明content_length时才手工设置(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)

	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Method:          http.MethodPost,
		Path:            "/x",
		Body:            "a=1&b=2",
		HeaderWhitelist: map[string]string{"content-length": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ContentLength=%d", srv.last().ContentLength)
	if got := srv.last().ContentLength; got != int64(len("a=1&b=2")) {
		t.Fatalf("ContentLength = %d, want %d", got, len("a=1&b=2"))
	}
}

// ─────────────────────────────── 请求体编码 ───────────────────────────────

func TestBody_编码矩阵(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string 原样保序", "z=1&a=2", "z=1&a=2"},
		{"bytes 原样", []byte("raw-bytes"), "raw-bytes"},
		{"url.Values 表单编码", url.Values{"b": {"2"}, "a": {"1"}}, "a=1&b=2"},
		{"map JSON", map[string]string{"k": "v"}, `{"k":"v"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
			c := newClient(t, srv.Server, nil)
			if _, err := c.PostForm(context.Background(), "/x", tc.in); err != nil {
				t.Fatal(err)
			}
			<-srv.mu
			got := string(srv.bodies[len(srv.bodies)-1])
			srv.mu <- struct{}{}
			t.Logf("%s → %q", tc.name, got)
			if got != tc.want {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────── 解压矩阵 ───────────────────────────────

func TestBodyDecode_四种压缩(t *testing.T) {
	const payload = `{"msg":"压缩内容 compressed body"}`
	cases := []struct {
		encoding string
		compress func([]byte) []byte
	}{
		{"gzip", func(b []byte) []byte {
			var buf bytes.Buffer
			w := gzip.NewWriter(&buf)
			_, _ = w.Write(b)
			_ = w.Close()
			return buf.Bytes()
		}},
		{"deflate", func(b []byte) []byte {
			var buf bytes.Buffer
			w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
			_, _ = w.Write(b)
			_ = w.Close()
			return buf.Bytes()
		}},
		{"br", func(b []byte) []byte {
			var buf bytes.Buffer
			w := brotli.NewWriter(&buf)
			_, _ = w.Write(b)
			_ = w.Close()
			return buf.Bytes()
		}},
		{"zstd", func(b []byte) []byte {
			var buf bytes.Buffer
			w, _ := zstd.NewWriter(&buf)
			_, _ = w.Write(b)
			_ = w.Close()
			return buf.Bytes()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.encoding, func(t *testing.T) {
			srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-encoding", tc.encoding)
				_, _ = w.Write(tc.compress([]byte(payload)))
			})
			c := newClient(t, srv.Server, nil)
			body, err := c.Get(context.Background(), "/x", nil)
			t.Logf("%s → %q err=%v", tc.encoding, body, err)
			if err != nil {
				t.Fatalf("%s: %v", tc.encoding, err)
			}
			if string(body) != payload {
				t.Fatalf("%s 解压结果 = %q, want %q", tc.encoding, body, payload)
			}
		})
	}
}

func TestBodyDecode_无content_encoding原样返回(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain"))
	})
	c := newClient(t, srv.Server, nil)
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("plain body=%q err=%v", body, err)
	if err != nil || string(body) != "plain" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

// ─────────────────────────────── 重试行为 ───────────────────────────────

func TestRetry_默认共四次尝试后返回RetryableError(t *testing.T) {
	var hits atomic.Int32
	// 用一个「接受连接后立刻断开」的服务器制造可重试的网络错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("no hijacker")
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close() // 直接断开 → 客户端侧 EOF
	}))
	defer srv.Close()

	c := newClient(t, srv, nil)
	_, err := c.Get(context.Background(), "/x", nil)
	if err == nil {
		t.Fatal("want error")
	}
	var re *kiterrors.RetryableError
	t.Logf("err=%v (%T) hits=%d", err, err, hits.Load())
	if !errors.As(err, &re) {
		t.Fatalf("err = %T %v, want *RetryableError", err, err)
	}
	if re.Attempts != 4 {
		t.Fatalf("Attempts = %d, want 4(1 次首发 + 3 次重试)", re.Attempts)
	}
	if got := hits.Load(); got != 4 {
		t.Fatalf("服务端收到 %d 次，want 4", got)
	}
}

func TestRetry_指数退避时序下界(t *testing.T) {
	// 锁定 200/400/800ms 的默认退避：总耗时至少 1.4s。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	c, err := interceptor.NewClient(httpx.Options{
		Headers: httpx.StaticHeaders{Base: srv.URL},
		Retry:   httpx.WithRetry(3, 200*time.Millisecond, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _ = c.Get(context.Background(), "/x", nil)
	elapsed := time.Since(start)
	t.Logf("elapsed=%v", elapsed)
	if elapsed < 1400*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 1.4s(200+400+800)", elapsed)
	}
}

func TestRetry_退避封顶生效(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	c, err := interceptor.NewClient(httpx.Options{
		Headers: httpx.StaticHeaders{Base: srv.URL},
		Retry:   httpx.WithRetry(3, 200*time.Millisecond, 250*time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _ = c.Get(context.Background(), "/x", nil)
	elapsed := time.Since(start)
	t.Logf("elapsed=%v cap=250ms", elapsed)
	// 封顶 250ms → 200+250+250=700ms 上下，明显小于不封顶的 1.4s
	if elapsed > 1200*time.Millisecond {
		t.Fatalf("elapsed = %v, 退避封顶未生效", elapsed)
	}
}

func TestRetry_不可重试错误立即返回(t *testing.T) {
	var hits atomic.Int32
	c, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://example.invalid"},
		Retry: &httpx.RetryPolicy{
			MaxRetries:  3,
			BaseBackoff: time.Millisecond,
			IsRetryable: func(error) bool { return false }, // 一律不可重试
		},
		Interceptors: httpx.Replace(interceptor.DefaultChain(), httpx.IsTerminal,
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				hits.Add(1)
				return nil, &httpx.TransportError{Err: fmt.Errorf("boom")}
			})),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/x", nil)
	t.Logf("non-retryable err=%v hits=%d", err, hits.Load())
	if err == nil {
		t.Fatal("want error")
	}
	var re *kiterrors.RetryableError
	if errors.As(err, &re) {
		t.Fatalf("不可重试错误不该被包成 RetryableError: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("尝试了 %d 次，不可重试错误应只发一次", got)
	}
}

func TestRetry_非传输层错误原样穿透(t *testing.T) {
	sentinel := errors.New("构头失败")
	var hits atomic.Int32
	c, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://example.invalid"},
		Retry:   httpx.WithRetry(3, time.Millisecond, 0),
		Interceptors: httpx.Replace(interceptor.DefaultChain(), httpx.IsTerminal,
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				hits.Add(1)
				return nil, sentinel // 未包成 TransportError
			})),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/x", nil)
	t.Logf("non-transport err=%v hits=%d", err, hits.Load())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 原样穿透 sentinel", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("尝试了 %d 次，非传输层错误应只发一次", got)
	}
}

func TestRetry_ctx取消时立即返回(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	c, err := interceptor.NewClient(httpx.Options{
		Headers: httpx.StaticHeaders{Base: srv.URL},
		Retry:   httpx.WithRetry(5, 300*time.Millisecond, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = c.Get(ctx, "/x", nil)
	t.Logf("ctx cancel err=%v elapsed=%v", err, time.Since(start))
	if err == nil {
		t.Fatal("want error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("elapsed = %v，ctx 取消后应立即返回而非跑完退避", elapsed)
	}
}

func TestRetry_每次重试重建请求头(t *testing.T) {
	// 会话状态在重试之间变化时，第二次尝试必须带新值 —— 这是 bridge 位于 retry 之内的意义。
	var n atomic.Int32
	hp := &countingHeaders{base: ""}
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	hp.base = srv.URL

	c, err := interceptor.NewClient(httpx.Options{Headers: hp, Retry: httpx.WithRetry(3, time.Millisecond, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("x-seq=%q", srv.last().Header.Get("x-seq"))
	if got := srv.last().Header.Get("x-seq"); got != "2" {
		t.Fatalf("x-seq = %q, want 2(重试时重新构头)", got)
	}
}

type countingHeaders struct {
	base string
	n    atomic.Int32
}

func (h *countingHeaders) BuildHeaders(context.Context) map[string]string {
	return map[string]string{"x-seq": fmt.Sprint(h.n.Add(1))}
}
func (h *countingHeaders) BaseURL() string { return h.base }

// ─────────────────────────────── 缓存时机 ───────────────────────────────

func TestSnapshot_状态码与响应头在重试跑完后才缓存(t *testing.T) {
	var n atomic.Int32
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		w.Header().Set("x-final", "yes")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("done"))
	})
	c := newClient(t, srv.Server, nil)

	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d x-final=%q", c.SnapshotResponseStatusCode(), c.SnapshotResponseHeaders().Get("x-final"))
	if got := c.SnapshotResponseStatusCode(); got != 201 {
		t.Fatalf("status = %d, want 201", got)
	}
	if got := c.SnapshotResponseHeaders().Get("x-final"); got != "yes" {
		t.Fatalf("x-final = %q", got)
	}
}

func TestResponseHeaderCache_非2xx也回写状态(t *testing.T) {
	// 服务端在 4xx 上照样可能下发新凭证，只在 2xx 回写会把它们丢掉。
	var got string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-new-token", "t-123")
		w.WriteHeader(403)
		_, _ = w.Write([]byte("forbidden"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.OnResponseHeaders = func(_ context.Context, h http.Header) { got = h.Get("x-new-token") }
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("on403 token=%q status=%d", got, c.SnapshotResponseStatusCode())
	if got != "t-123" {
		t.Fatalf("回写回调未在 403 上触发, got %q", got)
	}
}

// ─────────────────────────────── 链的顺序与编辑 ───────────────────────────────

func TestChain_执行顺序为外到内(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var order []string
	mark := func(name string) httpx.Interceptor {
		return httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
			order = append(order, "in:"+name)
			resp, err := ch.Proceed()
			order = append(order, "out:"+name)
			return resp, err
		})
	}
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), mark("A"), mark("B"))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"in:A", "in:B", "out:B", "out:A"}
	t.Logf("order=%v", order)
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestChain_SpliceBeforeTerminal包住每次真实发送(t *testing.T) {
	var sends atomic.Int32
	var n atomic.Int32
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) <= 2 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	counter := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		sends.Add(1)
		return ch.Proceed()
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(interceptor.DefaultChain(), counter)
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("sends=%d", sends.Load())
	if got := sends.Load(); got != 3 {
		t.Fatalf("拦截器被调用 %d 次，want 3(每次真实发送各一次)", got)
	}
}

func TestChain_WithChain保留最外层旁路观察层(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var captured int
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(*httpx.Transaction) { captured++ }))
	})

	derived := c.WithChain(interceptor.NoRedirectChain())
	if _, err := derived.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("captured=%d", captured)
	if captured != 1 {
		t.Fatalf("旁路 sink 在派生子链上被调用 %d 次，want 1", captured)
	}
}

func TestChain_WithChain独立缓存不污染父client(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/parent" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(418)
		}
		_, _ = w.Write([]byte("x"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.Get(context.Background(), "/parent", nil); err != nil {
		t.Fatal(err)
	}
	derived := c.WithChain(interceptor.DefaultChain())
	if _, err := derived.Get(context.Background(), "/child", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("parent=%d child=%d", c.SnapshotResponseStatusCode(), derived.SnapshotResponseStatusCode())
	if got := c.SnapshotResponseStatusCode(); got != 200 {
		t.Fatalf("父 client 状态被子链污染: %d", got)
	}
	if got := derived.SnapshotResponseStatusCode(); got != 418 {
		t.Fatalf("子 client 状态 = %d, want 418", got)
	}
}

func TestChain_NoRedirect把302原样交出(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/from" {
			w.Header().Set("location", "/to")
			w.Header().Set("set-cookie", "sid=abc; Path=/")
			w.WriteHeader(302)
			return
		}
		_, _ = w.Write([]byte("followed"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.NoRedirectChain()
	})
	body, err := c.Get(context.Background(), "/from", nil)
	t.Logf("302 body=%q status=%d location=%q", body, c.SnapshotResponseStatusCode(), c.SnapshotResponseHeaders().Get("location"))
	if err != nil {
		t.Fatalf("302 + 空体不该报错(默认链无状态语义层): %v", err)
	}
	if string(body) != "" {
		t.Fatalf("body = %q, want 空", body)
	}
	if got := c.SnapshotResponseStatusCode(); got != 302 {
		t.Fatalf("status = %d, want 302", got)
	}
	if got := c.SnapshotResponseHeaders().Get("location"); got != "/to" {
		t.Fatalf("location = %q", got)
	}
}

// ─────────────────────────────── 可选拦截器 ───────────────────────────────

func TestStatusSemantics_非2xx空体报错非空体放行(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"空体报错", "", true},
		{"非空体放行", `{"e":1}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
				_, _ = w.Write([]byte(tc.body))
			})
			c := newClient(t, srv.Server, func(o *httpx.Options) {
				o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewStatusSemanticsInterceptor(nil))
			})
			_, err := c.Get(context.Background(), "/x", nil)
			t.Logf("%s body=%q err=%v", tc.name, tc.body, err)
			if tc.wantErr {
				var se *kiterrors.HTTPStatusError
				if !errors.As(err, &se) || se.StatusCode != 500 {
					t.Fatalf("err = %v, want HTTPStatusError(500)", err)
				}
			} else if err != nil {
				t.Fatalf("非空体应放行, got %v", err)
			}
		})
	}
}

func TestStatusSemantics_限流文案规则包成RetryableError(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte("Please wait a few minutes before you try again"))
	})
	rule := httpx.RetryableTextRule([]string{"Please wait a few minutes"}, 572)
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewStatusSemanticsInterceptor(rule))
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("rate-limit err=%v (%T)", err, err)
	var re *kiterrors.RetryableError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v, want RetryableError", err)
	}
	var se *kiterrors.HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 429 {
		t.Fatalf("RetryableError 应包装 HTTPStatusError(429), got %v", err)
	}
}

func TestClassify_只在内层成功时才判定(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"fail","code":"banned"}`))
	})
	sentinel := errors.New("banned")
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.APIChain(func(_ int, body []byte) error {
			if strings.Contains(string(body), `"banned"`) {
				return sentinel
			}
			return nil
		})
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("classify err=%v is=%v", err, errors.Is(err, sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestHTMLText_提纯与错误页标记(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><script>var a=1</script></head><body><p>Hello</p><p>World</p></body></html>`))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor())
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("purified=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Hello World" {
		t.Fatalf("body = %q, want %q", body, "Hello World")
	}

	c2 := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("Hello"))
	})
	_, err = c2.Get(context.Background(), "/x", nil)
	t.Logf("marker err=%v", err)
	var se *kiterrors.HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 200 {
		t.Fatalf("err = %v, want HTTPStatusError(200)", err)
	}
}

func TestHTMLText_默认链不提纯保留script(t *testing.T) {
	// 默认即 raw 是本库与「默认提纯」实现的关键区别：页面里的 token 全在 script 里。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(`<html><script>var token="abc123"</script></html>`))
	})
	c := newClient(t, srv.Server, nil)
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("default html body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "abc123") {
		t.Fatalf("默认链不该剥掉 script, body = %q", body)
	}
}

func TestTransaction_快照字段完整(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-server", "kit")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("created"))
	})
	var txn *httpx.Transaction
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }))
	})
	if _, err := c.PostForm(context.Background(), "/create", "a=1"); err != nil {
		t.Fatal(err)
	}
	if txn != nil {
		t.Logf("txn method=%s status=%d req=%q resp=%q headers=%d", txn.Method, txn.Status, txn.ReqBody, txn.RespBody, len(txn.ReqHeaders))
	}
	if txn == nil {
		t.Fatal("sink 未被调用")
	}
	if txn.Method != http.MethodPost || txn.Status != 201 || txn.RespBody != "created" || txn.ReqBody != "a=1" {
		t.Fatalf("txn = %+v", txn)
	}
	if len(txn.ReqHeaders) == 0 {
		t.Fatal("ReqHeaders 为空，快照没拿到 bridge 构建的头")
	}
	if txn.ExitIP != "" || txn.ASN != "" {
		t.Fatalf("未传入出口信息时应为空: exit_ip=%q asn=%q", txn.ExitIP, txn.ASN)
	}
}

func TestTransaction_快照带上调用方传入的出口信息(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var txn *httpx.Transaction
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		// 自带 Transport 时 ProxyURL 不会真去拨号，只当快照元数据原样留下。
		o.Transport = &http.Transport{}
		o.ProxyURL = "socks5://gw.example:1080"
		o.ExitIP = "203.0.113.10"
		o.ASN = "AS64500"
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	if txn == nil {
		t.Fatal("sink 未被调用")
	}
	t.Logf("proxy=%q exit_ip=%q asn=%q", txn.Proxy, txn.ExitIP, txn.ASN)
	if txn.Proxy != "socks5://gw.example:1080" || txn.ExitIP != "203.0.113.10" || txn.ASN != "AS64500" {
		t.Fatalf("txn proxy=%q exit_ip=%q asn=%q", txn.Proxy, txn.ExitIP, txn.ASN)
	}
}

func TestRequestMutator_可删头(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(interceptor.DefaultChain(),
			interceptor.NewRequestMutatorInterceptor(func(r *httpx.Request) {
				delete(r.HTTPReq.Header, "user-agent")
			}))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("ua after delete=%q", srv.last().Header.Get("user-agent"))
	if got := srv.last().Header.Get("user-agent"); got != "" {
		t.Fatalf("user-agent = %q, want 已删除", got)
	}
}

// ─────────────────────────────── 契约与边界 ───────────────────────────────

func TestNew_非法代理URL报错(t *testing.T) {
	_, err := httpx.New(httpx.Options{
		Headers:  httpx.StaticHeaders{Base: "https://x.example"},
		ProxyURL: "ftp://1.2.3.4:1080",
	})
	t.Logf("proxy err=%v", err)
	if err == nil {
		t.Fatal("不支持的代理 scheme 应在构造期报错")
	}
	if !strings.HasPrefix(err.Error(), "httpx: build transport:") {
		t.Fatalf("err = %v, want 保留 build transport 前缀", err)
	}
	var ue *netproxy.UnsupportedProxySchemeError
	if !errors.As(err, &ue) || ue.Scheme != "ftp" {
		t.Fatalf("err = %v (%T), want *netproxy.UnsupportedProxySchemeError", err, err)
	}
}

func TestBuildHeaders返回nil时请求不发出(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.HeaderProviderFunc{Base: srv.URL, Build: func(context.Context) map[string]string { return nil }}
	})
	_, err := c.Get(context.Background(), "/x", nil)
	assertNilBuildHeaders(t, err, "httpx.HeaderProviderFunc")
	if srv.count() != 0 {
		t.Fatal("构头失败时不应发出网络请求")
	}
}

func TestFilterHeadersByWhitelist_大小写不敏感回退(t *testing.T) {
	all := map[string]string{"Content-Type": "application/json"}
	got := httpx.FilterHeadersByWhitelist(all, map[string]string{"content-type": ""})
	t.Logf("got=%v", got)
	if got["content-type"] != "application/json" {
		t.Fatalf("got = %v，应大小写不敏感命中构建值", got)
	}
}

func TestTruncateBodyForLog(t *testing.T) {
	// 行为锁：超限必须截断并带原始长度。边界表在 body_test 的 TestTruncateBodyForLog_边界。
	got := string(httpx.TruncateBodyForLog([]byte("abcdef"), 3))
	t.Logf("truncated=%q", got)
	if !strings.HasPrefix(got, "abc") || !strings.Contains(got, "total=6") {
		t.Fatalf("got = %q", got)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// ─────────────────────── 标准库特殊头的处理（HTTP/1.1 实测） ───────────────────────

func TestSpecialHeaders_UA不重复且无Go默认值(t *testing.T) {
	// 全小写写头在 HTTP/1.1 下会与标准库按规范化 key 输出的 User-Agent 撞车，
	// 表现为线上出现两条 UA（其中一条是 Go-http-client/1.1）。这条锁死修复。
	var raw []string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw = r.Header.Values("User-Agent")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("UA=%v", raw)
	if len(raw) != 1 || raw[0] != "kit-test/1.0" {
		t.Fatalf("User-Agent = %v, want 恰好一条 kit-test/1.0", raw)
	}
}

func TestSpecialHeaders_严格白名单不泄漏Go默认UA(t *testing.T) {
	var raw []string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw = r.Header.Values("User-Agent")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Path:            "/x",
		HeaderWhitelist: map[string]string{"accept": ""}, // 严格白名单里没有 user-agent
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("strict UA=%v", raw)
	if len(raw) != 0 {
		t.Fatalf("User-Agent = %v, 严格白名单未声明就不该发出任何 UA", raw)
	}
}

func TestSpecialHeaders_非严格模式保留标准库默认UA(t *testing.T) {
	// 反过来：没开严格白名单、构头也没给 UA 时，不去掉标准库默认 UA
	// —— 否则「随手 New 一个 client 打个接口」会因为没有 UA 被某些服务端拒绝。
	var raw []string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw = r.Header.Values("User-Agent")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"accept": "*/*"}}
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("default UA=%v", raw)
	if len(raw) != 1 || !strings.HasPrefix(raw[0], "Go-http-client") {
		t.Fatalf("User-Agent = %v, want 标准库默认值", raw)
	}
}

func TestSpecialHeaders_host头改写Host而非重复发送(t *testing.T) {
	var gotHost string
	var hostHeaderCount int
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		hostHeaderCount = len(r.Header.Values("Host"))
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"host": "virtual.example.com"}}
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("Host=%q headerCount=%d", gotHost, hostHeaderCount)
	if gotHost != "virtual.example.com" {
		t.Fatalf("Host = %q, want virtual.example.com", gotHost)
	}
	if hostHeaderCount != 0 {
		t.Fatalf("Host 不该再作为普通头重复发出, count = %d", hostHeaderCount)
	}
}

func TestSpecialHeaders_content_length不重复(t *testing.T) {
	var cl []string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		cl = r.Header.Values("Content-Length")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Method:          http.MethodPost,
		Path:            "/x",
		Body:            "a=1&b=2",
		HeaderWhitelist: map[string]string{"content-length": "7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Content-Length=%v", cl)
	if len(cl) > 1 {
		t.Fatalf("Content-Length 重复发送: %v", cl)
	}
}

func TestBuildHeaders返回的map不被就地改写(t *testing.T) {
	// origin/referer 注入与 extraHeaders 合并都必须发生在副本上，
	// 否则并发请求会互相污染 HeaderProvider 的内部状态。
	shared := map[string]string{"accept": "application/json"}
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.HeaderProviderFunc{
			Base:  srv.URL,
			Build: func(context.Context) map[string]string { return shared },
		}
	})
	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Path:         "/x",
		ExtraHeaders: map[string]string{"x-req": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("shared after request=%v", shared)
	if len(shared) != 1 {
		t.Fatalf("构头返回的 map 被就地改写: %v", shared)
	}
}

// ─────────────────────── 默认链自带的响应头缓存与会话回写 ───────────────────────

func TestDefaultChain_开箱即可读响应头快照(t *testing.T) {
	// SnapshotResponseHeaders 是 Client 上的公开方法，默认链必须让它真的有值 ——
	// 「方法在、但默认返回 nil」是最容易浪费人时间的一类设计。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-trace", "abc")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("x-trace=%q", c.SnapshotResponseHeaders().Get("x-trace"))
	if got := c.SnapshotResponseHeaders().Get("x-trace"); got != "abc" {
		t.Fatalf("x-trace = %q, 默认链应自带响应头缓存", got)
	}
}

func TestOnResponseHeaders_换链后仍然生效(t *testing.T) {
	// 会话状态回写是客户端级责任，不该因为 WithChain 换了链就悄悄失效 ——
	// 那会表现成「全程 200、业务就是不成功」，极难排查。
	var hits int
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-token", "t")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.OnResponseHeaders = func(context.Context, http.Header) { hits++ }
	})
	if _, err := c.Get(context.Background(), "/a", nil); err != nil {
		t.Fatal(err)
	}
	derived := c.WithChain(interceptor.NoRedirectChain())
	if _, err := derived.Get(context.Background(), "/b", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("OnResponseHeaders hits=%d", hits)
	if hits != 2 {
		t.Fatalf("回写被调用 %d 次，want 2(父链 + 派生子链各一次)", hits)
	}
}

func TestSnapshotRequestHeaders_不含抑制用的空UA且含host(t *testing.T) {
	// 整包快照要如实反映线上字节：那条空值 User-Agent 只是抑制标准库默认 UA 的开关，
	// 出现在快照里会让人对着抓包白排查；而 host 确实在线上，必须补回来。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var txn *httpx.Transaction
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot ua=%v host=%v", txn.ReqHeaders["user-agent"], txn.ReqHeaders["host"])
	if v, ok := txn.ReqHeaders["User-Agent"]; ok {
		t.Fatalf("快照里混进了规范化 User-Agent: %v", v)
	}
	if len(txn.ReqHeaders["user-agent"]) != 1 || txn.ReqHeaders["user-agent"][0] != "kit-test/1.0" {
		t.Fatalf("快照里的小写 user-agent = %v", txn.ReqHeaders["user-agent"])
	}
	if len(txn.ReqHeaders["host"]) != 1 || txn.ReqHeaders["host"][0] == "" {
		t.Fatalf("快照缺少 host: %v", txn.ReqHeaders)
	}
}

func TestSnapshotRequestHeaders_反映终端之前的改写(t *testing.T) {
	// 快照在终端拦截器里取，所以插在终端之前的改写层（签名、删头）都会被如实记录。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var txn *httpx.Transaction
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		chain := httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }))
		o.Interceptors = httpx.SpliceBeforeTerminal(chain,
			interceptor.NewRequestMutatorInterceptor(func(r *httpx.Request) {
				r.HTTPReq.Header["x-signature"] = []string{"sig-123"}
			}))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("x-signature=%v", txn.ReqHeaders["x-signature"])
	if got := txn.ReqHeaders["x-signature"]; len(got) != 1 || got[0] != "sig-123" {
		t.Fatalf("快照未反映终端之前的改写: %v", txn.ReqHeaders)
	}
}

func TestLogging_全量日志带上调用方传入的出口信息(t *testing.T) {
	buf := captureClientLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Transport = &http.Transport{}
		o.ProxyURL = "socks5://gw.example:1080"
		o.ExitIP = "203.0.113.10"
		o.ASN = "AS64500"
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	tx := findLogEvent(t, decodeLogRecords(t, buf), httpx.EventHTTPTransaction.Name())
	t.Logf("proxy=%v exit_ip=%v asn=%v", tx["proxy"], tx["exit_ip"], tx["asn"])
	if tx["proxy"] != "socks5://gw.example:1080" || tx["exit_ip"] != "203.0.113.10" || tx["asn"] != "AS64500" {
		t.Fatalf("proxy=%v exit_ip=%v asn=%v", tx["proxy"], tx["exit_ip"], tx["asn"])
	}
}

func TestLogging_摘要日志不含出口信息(t *testing.T) {
	buf := captureClientLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.LogSummaryOnly = true
		o.ExitIP = "203.0.113.10"
		o.ASN = "AS64500"
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	tx := findLogEvent(t, decodeLogRecords(t, buf), httpx.EventHTTPTransaction.Name())
	t.Logf("summary keys proxy=%v exit_ip=%v asn=%v", tx["proxy"], tx["exit_ip"], tx["asn"])
	if _, ok := tx["proxy"]; ok {
		t.Fatal("摘要日志不应含 proxy")
	}
	if _, ok := tx["exit_ip"]; ok {
		t.Fatal("摘要日志不应含 exit_ip")
	}
	if _, ok := tx["asn"]; ok {
		t.Fatal("摘要日志不应含 asn")
	}
}

func TestLogging_交易日志保留请求头原文(t *testing.T) {
	var buf bytes.Buffer
	logger.SetHandler(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() { logger.SetLogger(nil) })

	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "sid=secret")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.StaticHeaders{
			Base: srv.URL,
			Headers: map[string]string{
				"accept":        "application/json",
				"user-agent":    "kit-test/1.0",
				"authorization": "Bearer secret-token",
				"cookie":        "a=1",
			},
		}
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	log := buf.String()
	t.Logf("log contains secret-token=%v cookie=%v set-cookie=%v",
		strings.Contains(log, "secret-token"), strings.Contains(log, `"a=1"`), strings.Contains(log, "sid=secret"))
	if !strings.Contains(log, "secret-token") || !strings.Contains(log, `"a=1"`) || !strings.Contains(log, "sid=secret") {
		t.Fatalf("交易日志应保留头原文: %s", log)
	}
}

func decodeLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("解析日志 JSON: %v\n原文: %s", err, line)
		}
		records = append(records, record)
	}
	return records
}

func findLogEvent(t *testing.T, records []map[string]any, event string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["event"] == event {
			return record
		}
	}
	t.Fatalf("未找到 event=%q: %v", event, records)
	return nil
}

func captureClientLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetHandler(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() { logger.SetLogger(nil) })
	return &buf
}

func hijackClose(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, _ := hj.Hijack()
	_ = conn.Close()
}

func TestLogging_msg与event同值(t *testing.T) {
	buf := captureClientLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	want := []logger.Event{httpx.EventHTTPTransaction}
	for _, event := range want {
		name := event.Name()
		record := findLogEvent(t, records, name)
		t.Logf("event=%s msg=%v field=%v", name, record["msg"], record["event"])
		if record["msg"] != name || record["event"] != name {
			t.Fatalf("event=%q msg=%v field=%v", name, record["msg"], record["event"])
		}
	}
}

func TestLogging_重试日志保留请求头原文(t *testing.T) {
	buf := captureClientLogs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijackClose(w)
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv, func(o *httpx.Options) {
		o.Headers = httpx.StaticHeaders{
			Base: srv.URL,
			Headers: map[string]string{
				"authorization": "Bearer secret-token",
				"cookie":        "a=1",
				"user-agent":    "kit-test/1.0",
			},
		}
	})
	_, _ = c.Get(context.Background(), "/x", nil)
	log := buf.String()
	t.Logf("retry log=%s", log)
	records := decodeLogRecords(t, buf)
	retry := findLogEvent(t, records, httpx.EventHTTPRetry.Name())
	name := httpx.EventHTTPRetry.Name()
	if retry["msg"] != name || retry["event"] != name {
		t.Fatalf("retry msg=%v event=%v, want %q", retry["msg"], retry["event"], name)
	}
	if !strings.Contains(log, "secret-token") || !strings.Contains(log, `"a=1"`) {
		t.Fatalf("重试日志应保留头原文: %s", log)
	}
}

func TestLogging_重试成功msg与event同值(t *testing.T) {
	buf := captureClientLogs(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			hijackClose(w)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv, nil)
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	want := []logger.Event{
		httpx.EventHTTPRetry,
		httpx.EventHTTPRetrySucceeded,
		httpx.EventHTTPTransaction,
	}
	for _, event := range want {
		name := event.Name()
		record := findLogEvent(t, records, name)
		t.Logf("event=%s msg=%v field=%v", name, record["msg"], record["event"])
		if record["msg"] != name || record["event"] != name {
			t.Fatalf("event=%q msg=%v field=%v", name, record["msg"], record["event"])
		}
	}
}

func logEvents(records []map[string]any) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		if event, ok := record["event"].(string); ok && event != "" {
			out = append(out, event)
		}
	}
	return out
}

func jsonInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func findSpanEvent(t *testing.T, records []map[string]any, event, spanName string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["event"] == event && record["span_name"] == spanName {
			return record
		}
	}
	t.Fatalf("未找到 event=%q span_name=%q: %v", event, spanName, records)
	return nil
}

func TestLogging_成功事件树(t *testing.T) {
	buf := captureClientLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	const tid = "tid-fixed-1"
	if _, err := c.Get(logger.WithTraceID(context.Background(), tid), "/x", nil); err != nil {
		t.Fatal(err)
	}
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	wantEvents := []string{
		logger.EventSpanStart.Name(),
		httpx.EventHTTPTransaction.Name(),
		logger.EventSpanEnd.Name(),
	}
	if got := logEvents(records); strings.Join(got, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("events=%v, want %v", got, wantEvents)
	}
	start := findLogEvent(t, records, logger.EventSpanStart.Name())
	tx := findLogEvent(t, records, httpx.EventHTTPTransaction.Name())
	end := findLogEvent(t, records, logger.EventSpanEnd.Name())
	spanID, _ := start["span_id"].(string)
	if spanID == "" {
		t.Fatalf("span_id 为空: %v", start)
	}
	for _, record := range []map[string]any{start, tx, end} {
		t.Logf("event=%v trace_id=%v span_id=%v", record["event"], record["trace_id"], record["span_id"])
		if record["trace_id"] != tid {
			t.Fatalf("trace_id=%v, want %q", record["trace_id"], tid)
		}
		if record["span_id"] != spanID {
			t.Fatalf("span_id=%v, want %q", record["span_id"], spanID)
		}
	}
	if start["span_name"] != httpx.SpanHTTPRequest || end["span_name"] != httpx.SpanHTTPRequest {
		t.Fatalf("span_name start=%v end=%v", start["span_name"], end["span_name"])
	}
	if tx["msg"] != httpx.EventHTTPTransaction.Name() || tx["event"] != httpx.EventHTTPTransaction.Name() {
		t.Fatalf("transaction msg=%v event=%v", tx["msg"], tx["event"])
	}
	if jsonInt(end["status"]) != 200 {
		t.Fatalf("end status=%v", end["status"])
	}
	if _, ok := end["duration_ms"]; !ok {
		t.Fatalf("end 缺少 duration_ms: %v", end)
	}
}

func TestLogging_重试成功事件树(t *testing.T) {
	buf := captureClientLogs(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			hijackClose(w)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv, nil)
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	wantEvents := []string{
		logger.EventSpanStart.Name(),
		httpx.EventHTTPRetry.Name(),
		httpx.EventHTTPRetrySucceeded.Name(),
		httpx.EventHTTPTransaction.Name(),
		logger.EventSpanEnd.Name(),
	}
	if got := logEvents(records); strings.Join(got, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("events=%v, want %v", got, wantEvents)
	}
}

func TestLogging_错误仍成对span(t *testing.T) {
	buf := captureClientLogs(t)
	sentinel := errors.New("sentinel")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.SpliceBeforeTerminal(interceptor.DefaultChain(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				return nil, sentinel
			})),
	})
	_, err := c.Get(context.Background(), "/x", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v", err)
	}
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	start := findLogEvent(t, records, logger.EventSpanStart.Name())
	end := findLogEvent(t, records, logger.EventSpanEnd.Name())
	if start["span_name"] != httpx.SpanHTTPRequest || end["span_name"] != httpx.SpanHTTPRequest {
		t.Fatalf("span_name start=%v end=%v", start["span_name"], end["span_name"])
	}
	if end["error"] != sentinel.Error() {
		t.Fatalf("end error=%v, want %q", end["error"], sentinel.Error())
	}
	if _, ok := end["status"]; ok {
		t.Fatalf("失败路径不应伪造 status: %v", end)
	}
}

func TestLogging_父子span(t *testing.T) {
	buf := captureClientLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	const tid = "tid-parent-1"
	parentCtx, parentEnd := logger.StartSpan(logger.WithTraceID(context.Background(), tid), "biz.step")
	parentID := logger.SpanIDFromContext(parentCtx)
	if parentID == "" {
		t.Fatal("父 span_id 为空")
	}
	if _, err := c.Get(parentCtx, "/x", nil); err != nil {
		t.Fatal(err)
	}
	parentEnd()
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	httpStart := findSpanEvent(t, records, logger.EventSpanStart.Name(), httpx.SpanHTTPRequest)
	t.Logf("http start=%v parentID=%s", httpStart, parentID)
	if httpStart["parent_span_id"] != parentID {
		t.Fatalf("parent_span_id=%v, want %q", httpStart["parent_span_id"], parentID)
	}
	if httpStart["trace_id"] != tid {
		t.Fatalf("trace_id=%v, want %q", httpStart["trace_id"], tid)
	}
}

func TestLogging_ctx取消与deadline(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	cancel()

	var seenErr error
	var seenDeadline time.Time
	var hasDeadline bool
	probe := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		seenErr = ch.Request().Ctx.Err()
		seenDeadline, hasDeadline = ch.Request().Ctx.Deadline()
		return nil, seenErr
	})
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.SpliceBeforeTerminal(interceptor.DefaultChain(), probe),
	})
	_, _ = c.Get(ctx, "/x", nil)
	t.Logf("inner err=%v deadline=%v has=%v", seenErr, seenDeadline, hasDeadline)
	if !errors.Is(seenErr, context.Canceled) {
		t.Fatalf("内层 ctx.Err()=%v, want Canceled", seenErr)
	}
	if !hasDeadline || !seenDeadline.Equal(deadline) {
		t.Fatalf("deadline 被换掉: has=%v got=%v want=%v", hasDeadline, seenDeadline, deadline)
	}
}

func TestLogging_自组链无span(t *testing.T) {
	buf := captureClientLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Interceptors{
			interceptor.NewLoggingInterceptor(),
			interceptor.NewBodyDecodeInterceptor(),
			interceptor.NewBridgeInterceptor(),
			interceptor.NewCallServerInterceptor(),
		}
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	records := decodeLogRecords(t, buf)
	t.Logf("records=%v", records)
	for _, record := range records {
		if record["event"] == logger.EventSpanStart.Name() || record["event"] == logger.EventSpanEnd.Name() {
			t.Fatalf("自组链不应出现 span 节点: %v", records)
		}
	}
	tx := findLogEvent(t, records, httpx.EventHTTPTransaction.Name())
	if tx["msg"] != httpx.EventHTTPTransaction.Name() || tx["event"] != httpx.EventHTTPTransaction.Name() {
		t.Fatalf("自组链仍应打交易日志: %v", records)
	}
}

func TestLogging_tracing恢复父ctx(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	var before, after context.Context
	var innerSpan string
	outer := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		before = ch.Request().Ctx
		resp, err := ch.Proceed()
		after = ch.Request().Ctx
		return resp, err
	})
	inner := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		innerSpan = logger.SpanIDFromContext(ch.Request().Ctx)
		return ch.Proceed()
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(
			httpx.SpliceBeforeTerminal(interceptor.DefaultChain(), inner),
			outer,
		)
	})
	parent := logger.WithTraceID(context.Background(), "tid-restore")
	if _, err := c.Get(parent, "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("outer before span=%q after span=%q inner=%q",
		logger.SpanIDFromContext(before), logger.SpanIDFromContext(after), innerSpan)
	if before != after {
		t.Fatal("外层 Proceed 前后应是同一个父 ctx")
	}
	if logger.SpanIDFromContext(before) != "" || logger.SpanIDFromContext(after) != "" {
		t.Fatalf("外层不应看到已结束的 span_id: before=%q after=%q",
			logger.SpanIDFromContext(before), logger.SpanIDFromContext(after))
	}
	if innerSpan == "" {
		t.Fatal("内层应看到子 span_id")
	}
}

func TestTransaction_快照保留请求头原文(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "sid=secret")
		_, _ = w.Write([]byte("ok"))
	})
	var txn *httpx.Transaction
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.StaticHeaders{
			Base:    srv.URL,
			Headers: map[string]string{"authorization": "Bearer secret-token", "user-agent": "kit-test/1.0"},
		}
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	if txn == nil {
		t.Fatal("未收到快照")
	}
	t.Logf("txn authorization=%v", txn.ReqHeaders["authorization"])
	if got := txn.ReqHeaders["authorization"]; len(got) != 1 || got[0] != "Bearer secret-token" {
		t.Fatalf("Transaction 应保留原文: %v", txn.ReqHeaders["authorization"])
	}
}

func TestTransaction_快照头与后续改写隔离(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-server", "kit")
		w.Header().Add("set-cookie", "a=1")
		_, _ = w.Write([]byte("ok"))
	})
	var txn *httpx.Transaction
	var capturedReq *httpx.Request
	var capturedResp *httpx.Response
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }),
			httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
				resp, err := ch.Proceed()
				capturedReq = ch.Request()
				capturedResp = resp
				return resp, err
			}),
		)
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	if txn == nil || capturedReq == nil || capturedResp == nil {
		t.Fatal("未收到快照或链路引用")
	}

	if got := txn.ReqHeaders["user-agent"]; len(got) != 1 || got[0] != "kit-test/1.0" {
		t.Fatalf("快照 user-agent = %v", txn.ReqHeaders["user-agent"])
	}
	if got := txn.RespHeaders["X-Server"]; len(got) != 1 || got[0] != "kit" {
		t.Fatalf("快照 X-Server = %v", txn.RespHeaders["X-Server"])
	}

	// 改「原始」请求头验证快照已与之隔离。ReqHeaders 底层是 http.Header，本库刻意保留
	// 全小写原文 key，这里按同一约定转成裸 map 就地改写（直接用小写 key 索引 http.Header 会触发 SA1008）。
	rawReq := map[string][]string(capturedReq.ReqHeaders)
	rawReq["user-agent"][0] = "mutated"
	rawReq["x-injected"] = []string{"req"}
	capturedResp.Header.Set("x-server", "mutated")
	capturedResp.Header.Set("x-injected", "resp")
	if v := capturedResp.Header["Set-Cookie"]; len(v) > 0 {
		v[0] = "mutated-cookie"
	}

	t.Logf("after mutate req=%v resp=%v", txn.ReqHeaders["user-agent"], txn.RespHeaders["X-Server"])
	if got := txn.ReqHeaders["user-agent"]; len(got) != 1 || got[0] != "kit-test/1.0" {
		t.Fatalf("ReqHeaders 被后续改写污染: %v", txn.ReqHeaders)
	}
	if _, ok := txn.ReqHeaders["x-injected"]; ok {
		t.Fatalf("ReqHeaders 被注入新键: %v", txn.ReqHeaders)
	}
	if got := txn.RespHeaders["X-Server"]; len(got) != 1 || got[0] != "kit" {
		t.Fatalf("RespHeaders 被后续改写污染: %v", txn.RespHeaders)
	}
	if _, ok := txn.RespHeaders["x-injected"]; ok {
		t.Fatalf("RespHeaders 被注入新键: %v", txn.RespHeaders)
	}
	if got := txn.RespHeaders["Set-Cookie"]; len(got) != 1 || got[0] != "a=1" {
		t.Fatalf("RespHeaders 切片被就地改写: %v", txn.RespHeaders["Set-Cookie"])
	}
}
