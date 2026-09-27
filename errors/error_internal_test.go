package errors

import (
	stderrors "errors"
	"testing"
)

// TestProbe_只作遍历目标 probe 只作为 errors.As 的 target：文案固定，
// *Error.As 对非 probe 目标返回 false，不干扰标准 As 语义。
func TestProbe_只作遍历目标(t *testing.T) {
	if got := (probe{}).Error(); got != "errors: traversal probe" {
		t.Fatalf("Error()=%q", got)
	}
	e := &Error{Kind: NewKind("demo.a")}
	var other *HTTPStatusError
	if e.As(&other) || stderrors.As(e, &other) {
		t.Fatal("非 probe 目标不应命中")
	}
	var nilErr *Error
	if nilErr.As(&probe{visit: func(*Error) bool { return true }}) {
		t.Fatal("nil 接收者应返回 false")
	}
}
