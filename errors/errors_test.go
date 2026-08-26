package errors_test

import (
	stderrors "errors"
	"fmt"
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
)

func TestRetryableError_文案与解包(t *testing.T) {
	cause := stderrors.New("connection reset by peer")
	err := &kiterrors.RetryableError{Err: cause, Attempts: 4, LastError: cause}

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
		{"业务错误不重试", stderrors.New("invalid parameter"), false},
		{"包装过的 RetryableError", fmt.Errorf("wrap: %w", &kiterrors.RetryableError{Err: stderrors.New("x"), Attempts: 1}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := kiterrors.IsRetryableNetworkError(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegisterRetryableKeywords(t *testing.T) {
	err := stderrors.New("upstream proxy exhausted")
	if kiterrors.IsRetryableNetworkError(err) {
		t.Fatal("前置条件不成立：该文案本不该命中")
	}
	kiterrors.RegisterRetryableKeywords("Proxy Exhausted") // 大小写不敏感
	if !kiterrors.IsRetryableNetworkError(err) {
		t.Fatal("扩展关键词后应命中")
	}
	if len(kiterrors.RetryableKeywords()) == 0 {
		t.Fatal("快照不该为空")
	}
}

func TestHTTPStatusError(t *testing.T) {
	err := &kiterrors.HTTPStatusError{StatusCode: 503, Body: []byte("down")}
	if !kiterrors.IsHTTPStatus(fmt.Errorf("wrap: %w", err), 503) {
		t.Fatal("应能穿过包装识别状态码")
	}
	if kiterrors.IsHTTPStatus(err, 500) {
		t.Fatal("状态码不同不该命中")
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
	if !kiterrors.IsRetryableNetworkError(stderrors.New("dial failed: net.OpError while connecting")) {
		t.Fatal("含 net.OpError 的文案应判为可重试")
	}
}

func TestRegisterRetryableKeywords_空输入与空白条目被忽略(t *testing.T) {
	before := len(kiterrors.RetryableKeywords())
	kiterrors.RegisterRetryableKeywords()          // 空调用
	kiterrors.RegisterRetryableKeywords("", "   ") // 全是空白
	if after := len(kiterrors.RetryableKeywords()); after != before {
		t.Fatalf("关键词数 %d → %d，空条目不该入表", before, after)
	}
}
