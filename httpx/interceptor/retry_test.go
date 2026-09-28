package interceptor_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

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
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("不可重试 err=%v attempts=%d", err, term.n)
	if term.n != 1 {
		t.Fatalf("不可重试应只试 1 次，got %d", term.n)
	}
	if _, ok := errors.AsType[*kiterrors.RetryableError](err); ok {
		t.Fatal("不可重试不得包成 *RetryableError")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("应 %%w 保留底层 cause，err=%v", err)
	}
	var outer *kiterrors.Error
	if !errors.As(err, &outer) || outer.Op != "interceptor.send_request" || !errors.Is(outer.Err, cause) {
		t.Fatalf("不可重试错误应保留结构化步骤和底层原因，got %T %v", err, err)
	}
	if !strings.HasPrefix(err.Error(), "interceptor.send_request: ") {
		t.Fatalf("应走 sendRequest 包装，err=%v", err)
	}
}

func TestRetry_退避时ctx取消成RetryableError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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
	term := &transportFailTerminal{err: &url.Error{Op: "Get", URL: "https://x.example/x", Err: context.DeadlineExceeded}}
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
	if _, ok := errors.AsType[*kiterrors.RetryableError](err); ok {
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
	term := &errThenOKTerminal{err: &url.Error{Op: "Get", URL: "https://x.example/x", Err: context.DeadlineExceeded}, fails: 1}
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
	body, err := c.Get(t.Context(), "/x", nil)
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

// —— computeBackoff 的边界/对抗测试（经 export_test.go 暴露为 ComputeBackoff）。
// 契约（doc 承诺）：退避 = base*2^attempt，封顶 max；base>0 时结果必须 ∈ (0, max]（max>0），
// 绝不为 0 或负。指数退避在大 attempt 下不能整型溢出把 MaxBackoff 封顶绕过。

func TestComputeBackoff_正常翻倍(t *testing.T) {
	base := 200 * time.Millisecond
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 200 * time.Millisecond},
		{1, 400 * time.Millisecond},
		{2, 800 * time.Millisecond},
		{3, 1600 * time.Millisecond},
	}
	for _, c := range cases {
		got := interceptor.ComputeBackoff(base, 0, c.attempt) // max=0 不封顶
		t.Logf("base=%v attempt=%d → %v (want %v)", base, c.attempt, got, c.want)
		if got != c.want {
			t.Errorf("attempt=%d：得到 %v，应为 %v", c.attempt, got, c.want)
		}
	}
}

func TestComputeBackoff_封顶(t *testing.T) {
	base := 1 * time.Second
	cases := []struct {
		max     time.Duration
		attempt int
		desc    string
	}{
		{5 * time.Second, 10, "循环内提前触顶：base*1024 远超 5s"},
		{3 * time.Second, 2, "最后一次翻倍才越界：1s→2s→4s，循环后钳到 3s"},
	}
	for _, c := range cases {
		got := interceptor.ComputeBackoff(base, c.max, c.attempt)
		t.Logf("base=%v max=%v attempt=%d → %v（%s）", base, c.max, c.attempt, got, c.desc)
		if got != c.max {
			t.Errorf("%s：应封顶到 %v，得到 %v", c.desc, c.max, got)
		}
	}
}

// 核心 bug 用例：大 attempt 下 base*2^attempt 整型溢出，
// 退避不能变成 0 / 负数、也不能绕过 MaxBackoff。
func TestComputeBackoff_大attempt不溢出不绕过封顶(t *testing.T) {
	base, maxBackoff := 1*time.Second, 5*time.Second
	for _, attempt := range []int{34, 40, 62, 63, 64, 100, 200} {
		got := interceptor.ComputeBackoff(base, maxBackoff, attempt)
		t.Logf("base=%v max=%v attempt=%d → %v", base, maxBackoff, attempt, got)
		if got <= 0 {
			t.Errorf("attempt=%d：退避为 %v（≤0），指数退避整型溢出", attempt, got)
		}
		if got > maxBackoff {
			t.Errorf("attempt=%d：退避 %v 超过封顶 %v，MaxBackoff 被绕过", attempt, got, maxBackoff)
		}
	}
}

// 不封顶（max=0）时，大 attempt 也不能溢出成 0 / 负数——应饱和到一个正的大值。
func TestComputeBackoff_不封顶大attempt也不为负(t *testing.T) {
	base := 1 * time.Second
	for _, attempt := range []int{63, 64, 100, 200} {
		got := interceptor.ComputeBackoff(base, 0, attempt)
		t.Logf("base=%v max=0 attempt=%d → %v", base, attempt, got)
		if got <= 0 {
			t.Errorf("attempt=%d：不封顶时退避为 %v（≤0），整型溢出", attempt, got)
		}
	}
}

// base<=0（防御，normalizeRetry 已保证不会发生）返回 0，不 panic。
func TestComputeBackoff_base非正返回0(t *testing.T) {
	for _, base := range []time.Duration{0, -1 * time.Second} {
		got := interceptor.ComputeBackoff(base, 5*time.Second, 3)
		t.Logf("base=%v → %v", base, got)
		if got != 0 {
			t.Errorf("base=%v：应返回 0，得到 %v", base, got)
		}
	}
}

// 超大 max（接近 int64 上限）时，翻倍会先触发溢出保护而非 backoff>=max，结果仍 ∈ (0, max]。
func TestComputeBackoff_超大max溢出前钳住(t *testing.T) {
	base := 1 * time.Second
	maxBackoff := time.Duration(math.MaxInt64)
	got := interceptor.ComputeBackoff(base, maxBackoff, 200)
	t.Logf("base=%v max=%v attempt=200 → %v", base, maxBackoff, got)
	if got <= 0 || got > maxBackoff {
		t.Errorf("应 ∈ (0, %v]，得到 %v", maxBackoff, got)
	}
}
