package errors_test

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
)

var (
	kindLogin = kiterrors.NewKind("demo.login_required")
	kindRate  = kiterrors.NewKind("demo.rate_limited")
)

// sliceKind 不可比较的 Kind 实现：IsKind 不得因 == 比较而 panic。
type sliceKind []string

// Name 实现 Kind。
func (k sliceKind) Name() string { return fmt.Sprint([]string(k)) }

func TestError_Error各段组合(t *testing.T) {
	inner := stderrors.New("boom")
	cases := []struct {
		name string
		err  *kiterrors.Error
		want string
	}{
		{"全空", &kiterrors.Error{}, ""},
		{"只有 Op", &kiterrors.Error{Op: "svc.get"}, "svc.get"},
		{"只有 Kind", &kiterrors.Error{Kind: kindLogin}, "demo.login_required"},
		{"只有 Err", &kiterrors.Error{Err: inner}, "boom"},
		{"只有 Attrs", &kiterrors.Error{Attrs: []slog.Attr{slog.String("field", "user_id")}}, "field=user_id"},
		{"全部", &kiterrors.Error{
			Op: "svc.get", Kind: kindRate,
			Attrs: []slog.Attr{slog.Int("status", 429), slog.String("step", "send")},
			Err:   inner,
		}, "svc.get: demo.rate_limited: status=429 step=send: boom"},
		{"空名 Kind 不输出", &kiterrors.Error{Op: "svc.get", Kind: kiterrors.NewKind(""), Err: inner}, "svc.get: boom"},
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
	var e *kiterrors.Error
	if got := e.Error(); got != "<nil>" {
		t.Fatalf("nil Error()=%q", got)
	}
	if e.Unwrap() != nil {
		t.Fatal("nil Unwrap 应为 nil")
	}
}

func TestError_Unwrap穿透(t *testing.T) {
	status := &kiterrors.HTTPStatusError{StatusCode: 404}
	err := fmt.Errorf("outer: %w", &kiterrors.Error{Op: "svc.get", Err: status})
	got, ok := stderrors.AsType[*kiterrors.HTTPStatusError](err)
	if !ok || got.StatusCode != 404 {
		t.Fatalf("应穿透取到 HTTPStatusError：ok=%v got=%v", ok, got)
	}
	if e, ok := stderrors.AsType[*kiterrors.Error](err); !ok || e.Op != "svc.get" {
		t.Fatalf("应取到 *Error：%v", e)
	}
}

func TestKindOf(t *testing.T) {
	inner := &kiterrors.Error{Op: "inner", Kind: kindLogin}
	cases := []struct {
		name string
		err  error
		want kiterrors.Kind
	}{
		{"nil", nil, nil},
		{"普通错误", stderrors.New("x"), nil},
		{"无 Kind 的 *Error", &kiterrors.Error{Op: "a"}, nil},
		{"外层无 Kind 继承内层", &kiterrors.Error{Op: "outer", Err: inner}, kindLogin},
		{"外层有 Kind 优先", &kiterrors.Error{Op: "outer", Kind: kindRate, Err: inner}, kindRate},
		{"穿过其它包装", &kiterrors.RetryableError{Err: inner, Attempts: 1, LastError: inner}, kindLogin},
		{"穿过 errors.Join", stderrors.Join(stderrors.New("x"), inner), kindLogin},
		{"typed-nil *Error", (*kiterrors.Error)(nil), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kiterrors.KindOf(c.err); got != c.want {
				t.Fatalf("KindOf=%v, want %v", got, c.want)
			}
		})
	}
}

func TestIsKind(t *testing.T) {
	inner := &kiterrors.Error{Kind: kindLogin}
	outer := &kiterrors.Error{Op: "outer", Kind: kindRate, Err: inner}
	if !kiterrors.IsKind(outer, kindRate) || !kiterrors.IsKind(outer, kindLogin) {
		t.Fatal("链上任意一层的 Kind 都应命中")
	}
	wrapped := &kiterrors.RetryableError{Err: outer, Attempts: 1, LastError: outer}
	if !kiterrors.IsKind(wrapped, kindLogin) {
		t.Fatal("应穿过 RetryableError")
	}
	if kiterrors.IsKind(outer, kiterrors.NewKind("demo.other")) {
		t.Fatal("未出现的 Kind 不应命中")
	}
	if kiterrors.IsKind(nil, kindLogin) || kiterrors.IsKind(outer, nil) || kiterrors.IsKind(stderrors.New("x"), kindLogin) {
		t.Fatal("nil / 普通错误 / nil Kind 都应为 false")
	}
	bad := &kiterrors.Error{Kind: sliceKind{"a"}}
	if kiterrors.IsKind(bad, sliceKind{"a"}) {
		t.Fatal("不可比较的 Kind 应判为 false，且不得 panic")
	}
}

func TestAttrsOf(t *testing.T) {
	inner := &kiterrors.Error{Attrs: []slog.Attr{slog.String("field", "user_id")}}
	outer := &kiterrors.Error{Op: "outer", Attrs: []slog.Attr{slog.String("step", "send")}, Err: fmt.Errorf("mid: %w", inner)}
	got := kiterrors.AttrsOf(outer)
	if len(got) != 2 || got[0].Key != "step" || got[1].Key != "field" {
		t.Fatalf("AttrsOf 应从外到内合并：%v", got)
	}
	got[0] = slog.String("x", "y")
	if outer.Attrs[0].Key != "step" {
		t.Fatal("AttrsOf 返回值不得共享调用方底层数组")
	}
	if kiterrors.AttrsOf(nil) != nil || kiterrors.AttrsOf(stderrors.New("x")) != nil {
		t.Fatal("无 *Error 时应为 nil")
	}
}

// 多分支包装（fmt.Errorf 多个 %w、errors.Join）下，第二个分支上的 *Error 也要能判定到，
// 遍历顺序同 errors.Is / errors.As：前序深度优先。
func TestKindHelpers_遍历整棵错误树(t *testing.T) {
	a := &kiterrors.Error{Kind: kindLogin, Attrs: []slog.Attr{slog.String("a", "1")}}
	b := &kiterrors.Error{Kind: kindRate, Attrs: []slog.Attr{slog.String("b", "2")}}
	for name, err := range map[string]error{
		"多个 %w":       fmt.Errorf("x: %w: %w", a, b),
		"errors.Join": stderrors.Join(a, b),
	} {
		t.Run(name, func(t *testing.T) {
			if !kiterrors.IsKind(err, kindRate) || !kiterrors.IsKind(err, kindLogin) {
				t.Fatal("两个分支的 Kind 都应命中")
			}
			if got := kiterrors.KindOf(err); got != kindLogin {
				t.Fatalf("KindOf 应取前序第一个：%v", got)
			}
			got := kiterrors.AttrsOf(err)
			if len(got) != 2 || got[0].Key != "a" || got[1].Key != "b" {
				t.Fatalf("AttrsOf 应按前序合并两个分支：%v", got)
			}
		})
	}
}
