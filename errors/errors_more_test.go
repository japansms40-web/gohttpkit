package errors_test

// errors_more_test.go —— Unwrap / 快照隔离 / 并发注册 / 零值与空文案。

import (
	stderrors "errors"
	"fmt"
	"sync"
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
)

func TestRetryableError_Unwrap与As字段(t *testing.T) {
	cause := stderrors.New("connection reset by peer")
	last := stderrors.New("i/o timeout")
	err := &kiterrors.RetryableError{Err: cause, Attempts: 3, LastError: last}

	if stderrors.Unwrap(err) != cause {
		t.Fatalf("Unwrap() = %v, want cause", stderrors.Unwrap(err))
	}
	if !stderrors.Is(err, cause) {
		t.Fatal("errors.Is 应穿过 Unwrap 命中 Err")
	}
	var got *kiterrors.RetryableError
	if !stderrors.As(err, &got) {
		t.Fatal("errors.As 应解出 *RetryableError")
	}
	t.Logf("As Attempts=%d Err=%v Last=%v", got.Attempts, got.Err, got.LastError)
	if got.Attempts != 3 || got.Err != cause || got.LastError != last {
		t.Fatalf("字段不对: %+v", got)
	}
	msg := err.Error()
	if !contains(msg, "3") || !contains(msg, cause.Error()) || !contains(msg, last.Error()) {
		t.Fatalf("Error() 应同时含次数与两次错误: %q", msg)
	}
}

func TestRetryableKeywords_快照是副本(t *testing.T) {
	snap := kiterrors.RetryableKeywords()
	if len(snap) == 0 {
		t.Fatal("关键词表不应为空")
	}
	orig := snap[0]
	snap[0] = "definitely-not-a-real-keyword-xyz"
	t.Logf("改快照[0] %q → 注入串", orig)
	if kiterrors.IsRetryableNetworkError(stderrors.New("definitely-not-a-real-keyword-xyz")) {
		t.Fatal("改快照写回了内部表")
	}
	if !kiterrors.IsRetryableNetworkError(stderrors.New(orig)) {
		t.Fatal("内部表被快照改写破坏")
	}
}

func TestRegisterRetryableKeywords_并发读写不竞态(t *testing.T) {
	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan string, n*2)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			kiterrors.RegisterRetryableKeywords(fmt.Sprintf("concurrent-kw-%d", i))
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 32; j++ {
				_ = kiterrors.IsRetryableNetworkError(stderrors.New("timeout"))
				_ = kiterrors.RetryableKeywords()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
	if !kiterrors.IsRetryableNetworkError(stderrors.New("concurrent-kw-0")) {
		t.Fatal("并发追加的关键词应能被读到")
	}
	t.Logf("并发后表长=%d", len(kiterrors.RetryableKeywords()))
}

func TestIsRetryableNetworkError_空文案(t *testing.T) {
	got := kiterrors.IsRetryableNetworkError(stderrors.New(""))
	t.Logf("空文案 → %v", got)
	if got {
		t.Fatal("空文案不应命中关键词")
	}
}

func TestIsHTTPStatus_nil与无类型(t *testing.T) {
	if kiterrors.IsHTTPStatus(nil, 500) {
		t.Fatal("nil err 应为 false")
	}
	if kiterrors.IsHTTPStatus(stderrors.New("nope"), 500) {
		t.Fatal("链上无 *HTTPStatusError 应为 false")
	}
	t.Logf("nil / 普通 error 都不命中")
}

func TestHTTPStatusError_零值文案(t *testing.T) {
	err := &kiterrors.HTTPStatusError{}
	got := err.Error()
	t.Logf("零值 Error() = %q", got)
	want := kiterrors.FormatHTTPStatus(0, nil)
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestRegisterRetryableKeywords_重复追加不去重(t *testing.T) {
	before := len(kiterrors.RetryableKeywords())
	kiterrors.RegisterRetryableKeywords("dup-keyword-once")
	kiterrors.RegisterRetryableKeywords("dup-keyword-once")
	after := len(kiterrors.RetryableKeywords())
	t.Logf("重复追加 %d → %d", before, after)
	if after != before+2 {
		t.Fatalf("doc 承诺不去重，应 +2，得到 %d", after-before)
	}
}
