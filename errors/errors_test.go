package errors

import (
	stderrors "errors"
	"fmt"
	"sync"
	"testing"
)

func TestRetryableError_文案与解包(t *testing.T) {
	cause := stderrors.New("connection reset by peer")
	err := &RetryableError{Err: cause, Attempts: 4, LastError: cause}

	if !stderrors.Is(err, cause) {
		t.Fatal("应支持 errors.Is 解包到底层错误")
	}
	if got := err.Error(); got == "" || !contains(got, "4") {
		t.Fatalf("Error() = %q, 应含尝试次数", got)
	}
}

func TestIsRetryableNetworkError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"EOF", stderrors.New("EOF"), true},
		{"connection reset", stderrors.New("read tcp: connection reset by peer"), true},
		{"ctx 超时(不含 timeout 子串)", stderrors.New("context deadline exceeded"), true},
		{"i/o timeout", stderrors.New("dial tcp: i/o timeout"), true},
		{"DNS", stderrors.New("lookup x.example: no such host"), true},
		{"TLS 串包", stderrors.New("local error: tls: bad record MAC"), true},
		{"SOCKS", stderrors.New("socks connect tcp: unknown error"), true},
		{"net.OpError 文案", stderrors.New("dial failed: net.OpError while connecting"), true},
		{"业务错误不重试", stderrors.New("invalid parameter"), false},
		{"包装过的 RetryableError", fmt.Errorf("wrap: %w", &RetryableError{Err: stderrors.New("x"), Attempts: 1}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRetryableNetworkError(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegisterRetryableKeywords(t *testing.T) {
	err := stderrors.New("upstream proxy exhausted")
	if IsRetryableNetworkError(err) {
		t.Fatal("前置条件不成立：该文案本不该命中")
	}
	RegisterRetryableKeywords("Proxy Exhausted") // 大小写不敏感
	if !IsRetryableNetworkError(err) {
		t.Fatal("扩展关键词后应命中")
	}
	if len(RetryableKeywords()) == 0 {
		t.Fatal("快照不该为空")
	}
}

func TestHTTPStatusError(t *testing.T) {
	err := &HTTPStatusError{StatusCode: 503, Body: []byte("down")}
	if !IsHTTPStatus(fmt.Errorf("wrap: %w", err), 503) {
		t.Fatal("应能穿过包装识别状态码")
	}
	if IsHTTPStatus(err, 500) {
		t.Fatal("状态码不同不该命中")
	}
	if got, want := err.Error(), FormatHTTPStatus(503, []byte("down")); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestFormatHTTPStatus_与Error一致(t *testing.T) {
	if got := FormatHTTPStatus(404, []byte("nope")); got != "http error: 404, body: nope" {
		t.Fatalf("got %q", got)
	}
	var nilErr *HTTPStatusError
	if got := nilErr.Error(); !contains(got, "<nil>") {
		t.Fatalf("nil receiver Error() = %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestIsRetryableNetworkError_netOpError文案分支(t *testing.T) {
	// 有些包装层会把 net.OpError 的类型名直接写进文案，这条兜底规则专门接它。
	if !IsRetryableNetworkError(stderrors.New("dial failed: net.OpError while connecting")) {
		t.Fatal("含 net.OpError 的文案应判为可重试")
	}
}

func TestRegisterRetryableKeywords_空输入与空白条目被忽略(t *testing.T) {
	before := len(RetryableKeywords())
	RegisterRetryableKeywords()          // 空调用
	RegisterRetryableKeywords("", "   ") // 全是空白
	if after := len(RetryableKeywords()); after != before {
		t.Fatalf("关键词数 %d → %d，空条目不该入表", before, after)
	}
}

// —— 角度补充：Unwrap / 快照隔离 / 并发注册 / 零值与空文案。

func TestRetryableError_Unwrap与As字段(t *testing.T) {
	cause := stderrors.New("connection reset by peer")
	last := stderrors.New("i/o timeout")
	err := &RetryableError{Err: cause, Attempts: 3, LastError: last}

	// 这里断言指针同一，不是错误链匹配，故用 ==。
	//goland:noinspection GoDirectComparisonOfErrors
	if stderrors.Unwrap(err) != cause { //nolint:errorlint // 断言 Unwrap 返回放入的同一个 cause 指针，不是错误链匹配
		t.Fatalf("Unwrap() = %v, want cause", stderrors.Unwrap(err))
	}
	if !stderrors.Is(err, cause) {
		t.Fatal("errors.Is 应穿过 Unwrap 命中 Err")
	}
	var got *RetryableError
	if !stderrors.As(err, &got) {
		t.Fatal("errors.As 应解出 *RetryableError")
	}
	t.Logf("As Attempts=%d Err=%v Last=%v", got.Attempts, got.Err, got.LastError)
	// 同上：断言字段保存的是放入的同一个 error 指针。
	//goland:noinspection GoDirectComparisonOfErrors
	if got.Attempts != 3 || got.Err != cause || got.LastError != last { //nolint:errorlint // 断言字段保存的是当初放入的同一个 error 指针
		t.Fatalf("字段不对: %+v", got)
	}
	msg := err.Error()
	if !contains(msg, "3") || !contains(msg, cause.Error()) || !contains(msg, last.Error()) {
		t.Fatalf("Error() 应同时含次数与两次错误: %q", msg)
	}
}

func TestRetryableKeywords_快照是副本(t *testing.T) {
	snap := RetryableKeywords()
	if len(snap) == 0 {
		t.Fatal("关键词表不应为空")
	}
	orig := snap[0]
	snap[0] = "definitely-not-a-real-keyword-xyz"
	t.Logf("改快照[0] %q → 注入串", orig)
	if IsRetryableNetworkError(stderrors.New("definitely-not-a-real-keyword-xyz")) {
		t.Fatal("改快照写回了内部表")
	}
	if !IsRetryableNetworkError(stderrors.New(orig)) {
		t.Fatal("内部表被快照改写破坏")
	}
}

func TestRegisterRetryableKeywords_并发读写不竞态(t *testing.T) {
	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan string, n*2)
	for i := range n {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			RegisterRetryableKeywords(fmt.Sprintf("concurrent-kw-%d", i))
		}(i)
		go func() {
			defer wg.Done()
			for range 32 {
				_ = IsRetryableNetworkError(stderrors.New("timeout"))
				_ = RetryableKeywords()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
	if !IsRetryableNetworkError(stderrors.New("concurrent-kw-0")) {
		t.Fatal("并发追加的关键词应能被读到")
	}
	t.Logf("并发后表长=%d", len(RetryableKeywords()))
}

func TestIsRetryableNetworkError_空文案(t *testing.T) {
	got := IsRetryableNetworkError(stderrors.New(""))
	t.Logf("空文案 → %v", got)
	if got {
		t.Fatal("空文案不应命中关键词")
	}
}

func TestIsHTTPStatus_nil与无类型(t *testing.T) {
	if IsHTTPStatus(nil, 500) {
		t.Fatal("nil err 应为 false")
	}
	if IsHTTPStatus(stderrors.New("nope"), 500) {
		t.Fatal("链上无 *HTTPStatusError 应为 false")
	}
	t.Logf("nil / 普通 error 都不命中")
}

func TestHTTPStatusError_零值文案(t *testing.T) {
	err := &HTTPStatusError{}
	got := err.Error()
	t.Logf("零值 Error() = %q", got)
	want := FormatHTTPStatus(0, nil)
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestRegisterRetryableKeywords_重复追加不去重(t *testing.T) {
	before := len(RetryableKeywords())
	RegisterRetryableKeywords("dup-keyword-once")
	RegisterRetryableKeywords("dup-keyword-once")
	after := len(RetryableKeywords())
	t.Logf("重复追加 %d → %d", before, after)
	if after != before+2 {
		t.Fatalf("doc 承诺不去重，应 +2，得到 %d", after-before)
	}
}
