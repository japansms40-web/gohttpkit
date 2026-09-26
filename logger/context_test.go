package logger

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"testing"
)

func TestTraceIDFromContext_有则写出无则省略(t *testing.T) {
	buf := captureJSON(t, nil)

	Info(WithTraceID(t.Context(), "abc123"), "with trace")
	m := lastLine(t, buf)
	t.Logf("with trace → %v", m["trace_id"])
	if m["trace_id"] != "abc123" {
		t.Fatalf("trace_id = %v, want abc123", m["trace_id"])
	}

	buf.Reset()
	Info(t.Context(), "no trace")
	m = lastLine(t, buf)
	t.Logf("no trace keys trace_id=%v", m["trace_id"])
	if m["trace_id"] != nil {
		t.Fatalf("无 trace 的 ctx 不应输出 trace_id,got %v", m["trace_id"])
	}
}

func TestTraceIDFromContext_nil与空串(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"nil ctx", nil, ""},
		{"裸 Background", t.Context(), ""},
		{"显式空串", WithTraceID(t.Context(), ""), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TraceIDFromContext(c.ctx)
			t.Logf("TraceIDFromContext → %q", got)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestWithTraceID_nilContext不panic(t *testing.T) {
	//nolint:staticcheck // SA1012：本用例就是要验证传 nil context 不 panic 且能兜底，nil 正是被测契约
	ctx := WithTraceID(nil, "from-nil")
	got := TraceIDFromContext(ctx)
	t.Logf("WithTraceID(nil, from-nil) → %q", got)
	if got != "from-nil" {
		t.Fatalf("got %q", got)
	}
}

func TestEnsureTraceID_生成幂等且不覆盖调用方(t *testing.T) {
	ctx := EnsureTraceID(t.Context())
	tid := TraceIDFromContext(ctx)
	t.Logf("generated = %q", tid)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(tid) {
		t.Fatalf("生成的 trace_id 不是 16 位 hex: %q", tid)
	}

	ctx2 := EnsureTraceID(ctx)
	t.Logf("again = %q", TraceIDFromContext(ctx2))
	if TraceIDFromContext(ctx2) != tid {
		t.Fatalf("EnsureTraceID 不幂等: %q != %q", TraceIDFromContext(ctx2), tid)
	}

	pre := WithTraceID(t.Context(), "caller-set")
	got := TraceIDFromContext(EnsureTraceID(pre))
	t.Logf("caller-set after Ensure = %q", got)
	if got != "caller-set" {
		t.Fatalf("调用方 trace_id 被覆盖: %q", got)
	}

	empty := WithTraceID(t.Context(), "")
	filled := EnsureTraceID(empty)
	t.Logf("empty then Ensure = %q", TraceIDFromContext(filled))
	if TraceIDFromContext(filled) == "" {
		t.Fatal("空串 trace_id 应被兜底生成")
	}

	//nolint:staticcheck // SA1012：本用例就是要验证传 nil context 不 panic 且能兜底，nil 正是被测契约
	fromNil := EnsureTraceID(nil)
	t.Logf("EnsureTraceID(nil) = %q", TraceIDFromContext(fromNil))
	if TraceIDFromContext(fromNil) == "" {
		t.Fatal("nil ctx 也应生成 trace_id")
	}
}

func TestNewTraceID_不重复且格式正确(t *testing.T) {
	seen := make(map[string]struct{})
	re := regexp.MustCompile(`^[0-9a-f]{16}$`)
	for i := 0; i < 1000; i++ {
		id := NewTraceID()
		if !re.MatchString(id) {
			t.Fatalf("NewTraceID = %q 不是 16 位 hex", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("NewTraceID 重复: %q", id)
		}
		seen[id] = struct{}{}
	}
	t.Logf("1000 个 NewTraceID 无重复")
}

func TestWithAttrs_追加且不改父切片(t *testing.T) {
	buf := captureJSON(t, nil)

	parent := WithAttrs(t.Context(), slog.String("account_id", "u1"))
	child := WithAttrs(parent, slog.String("task_id", "t9"))

	Info(child, "both attrs")
	m := lastLine(t, buf)
	t.Logf("child → account_id=%v task_id=%v", m["account_id"], m["task_id"])
	if m["account_id"] != "u1" || m["task_id"] != "t9" {
		t.Fatalf("account_id=%v task_id=%v, want u1/t9", m["account_id"], m["task_id"])
	}

	buf.Reset()
	Info(parent, "parent only")
	m = lastLine(t, buf)
	t.Logf("parent → %v", m)
	if m["account_id"] != "u1" {
		t.Fatalf("父 ctx 丢失 account_id: %v", m)
	}
	if _, ok := m["task_id"]; ok {
		t.Fatalf("父 ctx 不应有 task_id: %v", m)
	}

	buf.Reset()
	Info(WithTraceID(child, "tid"), "all")
	m = lastLine(t, buf)
	t.Logf("trace+attrs → %v", m)
	if m["trace_id"] != "tid" || m["account_id"] != "u1" {
		t.Fatalf("trace+attrs 未同时生效: %v", m)
	}
}

func TestWithAttrs_空切片原样返回(t *testing.T) {
	parent := WithAttrs(t.Context(), slog.String("a", "1"))
	got := WithAttrs(parent)
	t.Logf("empty attrs same=%v", got == parent)
	if got != parent {
		t.Fatal("空 attrs 应原样返回同一父 ctx")
	}

	base := t.Context()
	if WithAttrs(base) != base {
		t.Fatal("无 attrs 应返回原 ctx")
	}
}

func TestWithAttrs_nilContext加字段(t *testing.T) {
	//nolint:staticcheck // SA1012：本用例就是要验证传 nil context 不 panic 且能兜底，nil 正是被测契约
	ctx := WithAttrs(nil, slog.String("k", "v"))
	got := attrsFromContext(ctx)
	t.Logf("WithAttrs(nil, k=v) → %v", got)
	if len(got) != 1 || got[0].Key != "k" {
		t.Fatalf("got %v", got)
	}
}

func TestWithAttrs_改子切片不影响父(t *testing.T) {
	parent := WithAttrs(t.Context(), slog.String("a", "1"))
	child := WithAttrs(parent, slog.String("b", "2"))
	got := attrsFromContext(child)
	if len(got) != 2 {
		t.Fatalf("child attrs = %v", got)
	}
	got[0] = slog.String("a", "HACK")
	parentAttrs := attrsFromContext(parent)
	t.Logf("parent=%v child after hack=%v", parentAttrs, got)
	if len(parentAttrs) != 1 || parentAttrs[0].Value.String() != "1" {
		t.Fatalf("改子切片不应污染父 ctx: %v", parentAttrs)
	}
}

func TestEnsureTraceID_不改入参Background(t *testing.T) {
	base := t.Context()
	got := EnsureTraceID(base)
	t.Logf("base tid=%q derived=%q", TraceIDFromContext(base), TraceIDFromContext(got))
	if TraceIDFromContext(base) != "" {
		t.Fatal("EnsureTraceID 不应改写入参 Background")
	}
	if TraceIDFromContext(got) == "" {
		t.Fatal("返回 ctx 应带 trace_id")
	}
}

func TestAttrsFromContext_nil返回nil(t *testing.T) {
	//nolint:staticcheck // SA1012：本用例就是要验证传 nil context 返回 nil 而非 panic，nil 正是被测契约
	got := attrsFromContext(nil)
	t.Logf("attrsFromContext(nil) = %v", got)
	if got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func TestConcurrentContextDerive_无数据竞争(t *testing.T) {
	buf := captureJSON(t, nil)
	base := WithAttrs(t.Context(), slog.String("base", "0"))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ctx := WithTraceID(WithAttrs(base, slog.Int("g", n)), "x")
				Info(ctx, "c")
			}
		}(i)
	}
	wg.Wait()
	t.Logf("8 goroutine ×100 次派生后 buf 行数约 %d", stringsCount(buf.String(), '\n'))
}

func stringsCount(s string, c byte) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			n++
		}
	}
	return n
}
