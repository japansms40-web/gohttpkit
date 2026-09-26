package interceptor_test

// angle_p0_test.go —— 按 TESTING.md §2 把 interceptor 包内行洞与未断言路径焊死：
// transaction 成功 sink、no_redirect 302、retry 三条出口、bridge 未跑分支、body Close 失败。
// 并发 #6 落在 httpx/concurrency_test.go（拦截器无共享可变状态），此处 out-of-scope。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/logger"
)

func captureInterceptorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetHandler(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() { logger.SetLogger(nil) })
	return &buf
}

func TestTransaction_成功sink字段与Clone隔离(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-server", "kit")
		w.Header().Add("set-cookie", "a=1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})
	var txn *httpx.Transaction
	var liveHeader http.Header
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Transport = &http.Transport{}
		o.ProxyURL = "socks5://gw.example:1080"
		o.ExitIP = "203.0.113.10"
		o.ASN = "AS64500"
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }),
			httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
				resp, err := ch.Proceed()
				if resp != nil {
					liveHeader = resp.Header
				}
				return resp, err
			}))
	})
	if _, err := c.PostForm(context.Background(), "/create", "a=1"); err != nil {
		t.Fatal(err)
	}
	if liveHeader != nil {
		liveHeader.Set("x-server", "mutated-after-sink")
	}
	if txn == nil {
		t.Fatal("成功路径 sink 必须回调")
	}
	t.Logf("txn method=%s status=%d req=%q resp=%q proxy=%q exit=%q asn=%q dur=%d x-server=%v",
		txn.Method, txn.Status, txn.ReqBody, txn.RespBody, txn.Proxy, txn.ExitIP, txn.ASN, txn.DurationMS, txn.RespHeaders["X-Server"])
	if txn.Method != http.MethodPost || txn.Status != http.StatusCreated {
		t.Fatalf("method/status = %s/%d", txn.Method, txn.Status)
	}
	if txn.ReqBody != "a=1" || txn.ReqBodyLen != 3 {
		t.Fatalf("ReqBody=%q len=%d", txn.ReqBody, txn.ReqBodyLen)
	}
	if txn.RespBody != "created" || txn.RespBodyLen != 7 {
		t.Fatalf("RespBody=%q len=%d", txn.RespBody, txn.RespBodyLen)
	}
	if txn.Proxy != "socks5://gw.example:1080" || txn.ExitIP != "203.0.113.10" || txn.ASN != "AS64500" {
		t.Fatalf("出口元数据丢失: %+v", txn)
	}
	if txn.DurationMS < 0 {
		t.Fatalf("DurationMS=%d", txn.DurationMS)
	}
	gotServer := ""
	for k, vs := range txn.RespHeaders {
		if strings.EqualFold(k, "x-server") && len(vs) > 0 {
			gotServer = vs[0]
		}
	}
	if gotServer != "kit" {
		t.Fatalf("Clone 隔离失败：后续改 Header 污染了快照, got %q headers=%v", gotServer, txn.RespHeaders)
	}
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
	body, err := c.Get(context.Background(), "/from", nil)
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

type transportFailTerminal struct {
	httpx.TerminalMarker
	err   error
	n     int
	after func()
}

func (t *transportFailTerminal) Intercept(*httpx.Chain) (*httpx.Response, error) {
	t.n++
	if t.after != nil {
		t.after()
	}
	return nil, &httpx.TransportError{Err: t.err}
}

type succeedOnRetryTerminal struct {
	httpx.TerminalMarker
	fails int
	n     int
}

func (t *succeedOnRetryTerminal) Intercept(*httpx.Chain) (*httpx.Response, error) {
	t.n++
	if t.n <= t.fails {
		return nil, &httpx.TransportError{Err: errors.New("connection reset by peer")}
	}
	return &httpx.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: []byte("ok")}, nil
}

func TestRetry_不可重试包装底层错误(t *testing.T) {
	cause := errors.New("tls: unknown alert")
	term := &transportFailTerminal{err: cause}
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Retry: &httpx.RetryPolicy{
			MaxRetries:  3,
			BaseBackoff: time.Millisecond,
			IsRetryable: func(error) bool { return false },
		},
		Interceptors: httpx.Interceptors{interceptor.NewRetryInterceptor(), term},
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("不可重试 err=%v attempts=%d", err, term.n)
	if term.n != 1 {
		t.Fatalf("不可重试应只试 1 次，got %d", term.n)
	}
	var re *kiterrors.RetryableError
	if errors.As(err, &re) {
		t.Fatal("不可重试不得包成 *RetryableError")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("应 %%w 保留底层 cause，err=%v", err)
	}
	if !strings.Contains(err.Error(), "failed to send request") {
		t.Fatalf("应走 sendRequest 包装，err=%v", err)
	}
}

func TestRetry_退避时ctx取消成RetryableError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	term := &transportFailTerminal{
		err:   errors.New("connection reset by peer"),
		after: cancel,
	}
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Retry:   httpx.WithRetry(3, time.Hour, 0),
		Interceptors: httpx.Interceptors{
			interceptor.NewRetryInterceptor(),
			term,
		},
	})
	_, err := c.Get(ctx, "/x", nil)
	t.Logf("ctx 取消 err=%v attempts=%d", err, term.n)
	var re *kiterrors.RetryableError
	if !errors.As(err, &re) {
		t.Fatalf("err=%v (%T)，要 *RetryableError", err, err)
	}
	if !errors.Is(re.Err, context.Canceled) {
		t.Fatalf("RetryableError.Err 应是 ctx.Err()，got %v", re.Err)
	}
	if re.Attempts != 1 {
		t.Fatalf("Attempts=%d, want 1（第一次失败后取消）", re.Attempts)
	}
	if term.n != 1 {
		t.Fatalf("取消后不应再发，attempts=%d", term.n)
	}
}

// errThenOKTerminal 前 fails 次返回 err 的传输错误，之后成功。
type errThenOKTerminal struct {
	httpx.TerminalMarker
	err   error
	fails int
	n     int
}

func (t *errThenOKTerminal) Intercept(*httpx.Chain) (*httpx.Response, error) {
	t.n++
	if t.n <= t.fails {
		return nil, &httpx.TransportError{Err: t.err}
	}
	return &httpx.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: []byte("ok")}, nil
}

func TestRetry_调用方ctx到期引起的失败不重试(t *testing.T) {
	buf := captureInterceptorLogs(t)
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	// 文案含 "context deadline exceeded"，默认关键词表判为可重试；但它正是调用方 ctx 到期造成的。
	term := &transportFailTerminal{err: fmt.Errorf("Get \"https://x.example/x\": %w", context.DeadlineExceeded)}
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Retry:        httpx.WithRetry(3, time.Hour, 0),
		Interceptors: httpx.Interceptors{interceptor.NewRetryInterceptor(), term},
	})
	_, err := c.Get(ctx, "/x", nil)
	t.Logf("调用方 ctx 到期 err=%v attempts=%d", err, term.n)
	if term.n != 1 {
		t.Fatalf("调用方 ctx 已到期不应重试，attempts=%d", term.n)
	}
	var re *kiterrors.RetryableError
	if errors.As(err, &re) {
		t.Fatalf("调用方 ctx 到期不得包成 *RetryableError，err=%v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应保留 context.DeadlineExceeded，err=%v", err)
	}
	if strings.Contains(buf.String(), httpx.EventHTTPRetry.Name()) {
		t.Fatalf("不应打 http.retry 事件，logs=%s", buf.String())
	}
}

func TestRetry_ctx仍存活时超时类错误照常重试(t *testing.T) {
	// 模拟 http.Client.Timeout：错误链含 DeadlineExceeded，但调用方 ctx 仍存活，属网络层超时，应重试。
	buf := captureInterceptorLogs(t)
	term := &errThenOKTerminal{err: fmt.Errorf("Get \"https://x.example/x\": %w", context.DeadlineExceeded), fails: 1}
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Retry:        httpx.WithRetry(3, time.Millisecond, 0),
		Interceptors: httpx.Interceptors{interceptor.NewRetryInterceptor(), term},
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("ctx 存活的超时 err=%v attempts=%d", err, term.n)
	if err != nil {
		t.Fatalf("第二次应成功，err=%v", err)
	}
	if string(body) != "ok" || term.n != 2 {
		t.Fatalf("body=%q attempts=%d，want ok / 2", body, term.n)
	}
	if !strings.Contains(buf.String(), httpx.EventHTTPRetry.Name()) {
		t.Fatalf("网络层超时应打 http.retry 事件，logs=%s", buf.String())
	}
}

func TestRetry_第二次成功打retrySucceeded(t *testing.T) {
	buf := captureInterceptorLogs(t)
	term := &succeedOnRetryTerminal{fails: 1}
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Retry:        httpx.WithRetry(2, time.Millisecond, 0),
		Interceptors: httpx.Interceptors{interceptor.NewRetryInterceptor(), term},
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("重试成功 body=%q err=%v attempts=%d logs=%s", body, err, term.n, buf.String())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body=%q", body)
	}
	if term.n != 2 {
		t.Fatalf("attempts=%d, want 2", term.n)
	}
	if !strings.Contains(buf.String(), "http.retry.succeeded") {
		t.Fatal("重试成功必须打 event=http.retry.succeeded")
	}
}

func TestBridge_POST体与白名单与空ExtraHeaders(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.DefaultChain()
	})
	_, err := c.Do(context.Background(), httpx.RequestSpec{
		Method:          http.MethodPost,
		Path:            "/x",
		Body:            []byte("hello!!"),
		HeaderWhitelist: map[string]string{"accept": "", "content-length": "7"},
		ExtraHeaders:    map[string]string{"x-skip": "", "x-keep": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	gotBody := srv.bodies[len(srv.bodies)-1]
	srv.mu <- struct{}{}
	t.Logf("method=%s body=%q cl=%d ua=%q accept=%q skip=%q keep=%q origin=%q",
		last.Method, gotBody, last.ContentLength, last.Header.Get("user-agent"), last.Header.Get("accept"),
		last.Header.Get("x-skip"), last.Header.Get("x-keep"), last.Header.Get("origin"))
	if last.Method != http.MethodPost || string(gotBody) != "hello!!" {
		t.Fatalf("POST 体没发出: method=%s body=%q", last.Method, gotBody)
	}
	if last.ContentLength != 7 {
		t.Fatalf("合法 content-length 应写入 Request.ContentLength，got %d", last.ContentLength)
	}
	if ua := last.Header.Get("User-Agent"); ua != "" && ua != "kit-test/1.0" {
		// 严格模式应抑制 Go 默认 UA（Go-http-client/1.1）
		if strings.Contains(ua, "Go-http-client") {
			t.Fatalf("严格白名单泄漏了 Go 默认 UA: %q", ua)
		}
	}
	if last.Header.Get("x-skip") != "" {
		t.Fatal("ExtraHeaders 空值应跳过")
	}
	if last.Header.Get("x-keep") != "1" {
		t.Fatalf("x-keep=%q", last.Header.Get("x-keep"))
	}
}

func TestBridge_DisableOriginReferer不注入(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.DisableOriginReferer = true
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	srv.mu <- struct{}{}
	t.Logf("origin=%q referer=%q", last.Header.Get("origin"), last.Header.Get("referer"))
	if last.Header.Get("origin") != "" || last.Header.Get("referer") != "" {
		t.Fatal("DisableOriginReferer 后不应注入 origin/referer")
	}
}

func TestBodyDecode_Close失败仍返回体(t *testing.T) {
	buf := captureInterceptorLogs(t)
	closeErr := errors.New("close pipe")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewBodyDecodeInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				return &httpx.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Raw:        &http.Response{Body: eofCloseFail{closeErr: closeErr}},
				}, nil
			}),
		},
	})
	// InterceptorFunc 不是终端……但 Proceed 会调用它并返回，不需要终端如果它是最后一层？
	// Chain.Proceed: 如果 index >= len，ChainExhausted。所以最后一层必须自己不 Proceed，或是终端。
	// InterceptorFunc 会作为最后一层被调用，只要它不 Proceed 就行。OK。
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("Close 失败 body=%q err=%v logs=%s", body, err, buf.String())
	if err != nil {
		t.Fatalf("Close 失败不应淹没已读体，err=%v", err)
	}
	if string(body) != "" {
		t.Fatalf("EOF 体应为空，got %q", body)
	}
	if !strings.Contains(buf.String(), "failed to close response body") {
		t.Fatal("Close 失败应打 logger.Error")
	}
}

type eofCloseFail struct{ closeErr error }

func (e eofCloseFail) Read([]byte) (int, error) { return 0, io.EOF }
func (e eofCloseFail) Close() error             { return e.closeErr }

func TestHTMLText_空marker忽略不误伤(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte("<html><body>Access Denied</body></html>"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("", ""))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("空 marker body=%q err=%v", body, err)
	if err != nil {
		t.Fatalf("空 marker 不应把正文当错误页: %v", err)
	}
	if !strings.Contains(string(body), "Access Denied") {
		t.Fatalf("应留下可见文案，got %q", body)
	}
}

func TestStatusCodeCache_成功写入快照(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("status snapshot=%d", c.SnapshotResponseStatusCode())
	if c.SnapshotResponseStatusCode() != http.StatusTeapot {
		t.Fatalf("Snapshot=%d, want 418", c.SnapshotResponseStatusCode())
	}
}

func TestResponseHeaderCache_多值SetCookie进hook(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("set-cookie", "a=1")
		w.Header().Add("set-cookie", "b=2")
		_, _ = w.Write([]byte("ok"))
	})
	var cookies []string
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.OnResponseHeaders = func(_ context.Context, h http.Header) {
			cookies = append([]string(nil), h.Values("set-cookie")...)
		}
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("hook cookies=%v snapshot=%v", cookies, c.SnapshotResponseHeaders().Values("set-cookie"))
	if len(cookies) != 2 {
		t.Fatalf("hook 应收到两条 Set-Cookie，got %v", cookies)
	}
}

func TestLogging_摘要2xx字段可被抓到(t *testing.T) {
	buf := captureInterceptorLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.LogSummaryOnly = true })
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("logs=%s", buf.String())
	var sawTxn bool
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec["event"] == "http.transaction" {
			sawTxn = true
		}
	}
	if !sawTxn {
		t.Fatal("摘要 2xx 仍应打 event=http.transaction")
	}
}
