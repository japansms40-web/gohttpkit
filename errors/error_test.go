package errors

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"testing"
)

var (
	kindLogin = NewKind("demo.login_required")
	kindRate  = NewKind("demo.rate_limited")
)

// sliceKind 不可比较的 Kind 实现：IsKind 不得因 == 比较而 panic。
type sliceKind []string

// Name 实现 Kind。
func (k sliceKind) Name() string { return fmt.Sprint([]string(k)) }

func TestError_Error各段组合(t *testing.T) {
	inner := stderrors.New("boom")
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{"全空", &Error{}, ""},
		{"只有 Op", &Error{Op: "svc.get"}, "svc.get"},
		{"只有 Kind", &Error{Kind: kindLogin}, "demo.login_required"},
		{"只有 Err", &Error{Err: inner}, "boom"},
		{"只有 Attrs", &Error{Attrs: []slog.Attr{slog.String("field", "user_id")}}, "field=user_id"},
		{"全部", &Error{
			Op: "svc.get", Kind: kindRate,
			Attrs: []slog.Attr{slog.Int("status", 429), slog.String("step", "send")},
			Err:   inner,
		}, "svc.get: demo.rate_limited: status=429 step=send: boom"},
		{"空名 Kind 不输出", &Error{Op: "svc.get", Kind: NewKind(""), Err: inner}, "svc.get: boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Error(); got != c.want {
				t.Fatalf("Error()=%q, want %q", got, c.want)
			}
		})
	}
}

func TestError_nil接收者(t *testing.T) {
	var e *Error
	if got := e.Error(); got != "<nil>" {
		t.Fatalf("nil Error()=%q", got)
	}
	if e.Unwrap() != nil {
		t.Fatal("nil Unwrap 应为 nil")
	}
}

func TestError_Unwrap穿透(t *testing.T) {
	status := &HTTPStatusError{StatusCode: 404}
	err := fmt.Errorf("outer: %w", &Error{Op: "svc.get", Err: status})
	got, ok := stderrors.AsType[*HTTPStatusError](err)
	if !ok || got.StatusCode != 404 {
		t.Fatalf("应穿透取到 HTTPStatusError：ok=%v got=%v", ok, got)
	}
	if e, ok := stderrors.AsType[*Error](err); !ok || e.Op != "svc.get" {
		t.Fatalf("应取到 *Error：%v", e)
	}
}

func TestKindOf(t *testing.T) {
	inner := &Error{Op: "inner", Kind: kindLogin}
	cases := []struct {
		name string
		err  error
		want Kind
	}{
		{"nil", nil, nil},
		{"普通错误", stderrors.New("x"), nil},
		{"无 Kind 的 *Error", &Error{Op: "a"}, nil},
		{"外层无 Kind 继承内层", &Error{Op: "outer", Err: inner}, kindLogin},
		{"外层有 Kind 优先", &Error{Op: "outer", Kind: kindRate, Err: inner}, kindRate},
		{"穿过其它包装", &RetryableError{Err: inner, Attempts: 1, LastError: inner}, kindLogin},
		{"穿过 errors.Join", stderrors.Join(stderrors.New("x"), inner), kindLogin},
		{"typed-nil *Error", (*Error)(nil), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := KindOf(c.err); got != c.want {
				t.Fatalf("KindOf=%v, want %v", got, c.want)
			}
		})
	}
}

func TestIsKind(t *testing.T) {
	inner := &Error{Kind: kindLogin}
	outer := &Error{Op: "outer", Kind: kindRate, Err: inner}
	if !IsKind(outer, kindRate) || !IsKind(outer, kindLogin) {
		t.Fatal("链上任意一层的 Kind 都应命中")
	}
	wrapped := &RetryableError{Err: outer, Attempts: 1, LastError: outer}
	if !IsKind(wrapped, kindLogin) {
		t.Fatal("应穿过 RetryableError")
	}
	if IsKind(outer, NewKind("demo.other")) {
		t.Fatal("未出现的 Kind 不应命中")
	}
	if IsKind(nil, kindLogin) || IsKind(outer, nil) || IsKind(stderrors.New("x"), kindLogin) {
		t.Fatal("nil / 普通错误 / nil Kind 都应为 false")
	}
	bad := &Error{Kind: sliceKind{"a"}}
	if IsKind(bad, sliceKind{"a"}) {
		t.Fatal("不可比较的 Kind 应判为 false，且不得 panic")
	}
}

func TestAttrsOf(t *testing.T) {
	inner := &Error{Attrs: []slog.Attr{slog.String("field", "user_id")}}
	outer := &Error{Op: "outer", Attrs: []slog.Attr{slog.String("step", "send")}, Err: fmt.Errorf("mid: %w", inner)}
	got := AttrsOf(outer)
	if len(got) != 2 || got[0].Key != "step" || got[1].Key != "field" {
		t.Fatalf("AttrsOf 应从外到内合并：%v", got)
	}
	got[0] = slog.String("x", "y")
	if outer.Attrs[0].Key != "step" {
		t.Fatal("AttrsOf 返回值不得共享调用方底层数组")
	}
	if AttrsOf(nil) != nil || AttrsOf(stderrors.New("x")) != nil {
		t.Fatal("无 *Error 时应为 nil")
	}
}

// 多分支包装（fmt.Errorf 多个 %w、errors.Join）下，第二个分支上的 *Error 也要能判定到，
// 遍历顺序同 errors.Is / errors.As：前序深度优先。
func TestKindHelpers_遍历整棵错误树(t *testing.T) {
	a := &Error{Kind: kindLogin, Attrs: []slog.Attr{slog.String("a", "1")}}
	b := &Error{Kind: kindRate, Attrs: []slog.Attr{slog.String("b", "2")}}
	for name, err := range map[string]error{
		"多个 %w":       fmt.Errorf("x: %w: %w", a, b),
		"errors.Join": stderrors.Join(a, b),
	} {
		t.Run(name, func(t *testing.T) {
			if !IsKind(err, kindRate) || !IsKind(err, kindLogin) {
				t.Fatal("两个分支的 Kind 都应命中")
			}
			if got := KindOf(err); got != kindLogin {
				t.Fatalf("KindOf 应取前序第一个：%v", got)
			}
			got := AttrsOf(err)
			if len(got) != 2 || got[0].Key != "a" || got[1].Key != "b" {
				t.Fatalf("AttrsOf 应按前序合并两个分支：%v", got)
			}
		})
	}
}

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
