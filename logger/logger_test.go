package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// TestMain 把默认 logger 指向 io.Discard,避免还原默认后的用例向 stdout 刷日志
func TestMain(m *testing.M) {
	fallback.Store(slog.New(wrapCtxHandler(slog.NewJSONHandler(io.Discard, nil))))
	os.Exit(m.Run())
}

// captureJSON 注入一个写入 buf 的 JSONHandler,返回 buf;测试结束自动还原
func captureJSON(t *testing.T, opts *slog.HandlerOptions) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, opts))
	t.Cleanup(func() { SetLogger(nil) })
	return buf
}

// lastLine 解析 buf 最后一行 JSON 日志
func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("解析日志 JSON 失败: %v\n原文: %s", err, buf.String())
	}
	return m
}

// Test_TraceID注入 trace_id 从 ctx 自动附加;无 trace 时不出现该键
func TestTraceIDInjection(t *testing.T) {
	buf := captureJSON(t, nil)

	Info(WithTraceID(context.Background(), "abc123"), "with trace")
	if m := lastLine(t, buf); m["trace_id"] != "abc123" {
		t.Fatalf("trace_id = %v, want abc123", m["trace_id"])
	}

	buf.Reset()
	Info(context.Background(), "no trace")
	if m := lastLine(t, buf); m["trace_id"] != nil {
		t.Fatalf("无 trace 的 ctx 不应输出 trace_id,got %v", m["trace_id"])
	}
}

// TestWithAttrs 通用字段自动附加;二次追加不覆盖;父 ctx 切片不被修改
func TestWithAttrs(t *testing.T) {
	buf := captureJSON(t, nil)

	parent := WithAttrs(context.Background(), slog.String("account_id", "u1"))
	child := WithAttrs(parent, slog.String("task_id", "t9"))

	Info(child, "both attrs")
	m := lastLine(t, buf)
	if m["account_id"] != "u1" || m["task_id"] != "t9" {
		t.Fatalf("account_id=%v task_id=%v, want u1/t9", m["account_id"], m["task_id"])
	}

	// 父 ctx 不受 child 追加影响(copy-on-write)
	buf.Reset()
	Info(parent, "parent only")
	m = lastLine(t, buf)
	if m["account_id"] != "u1" {
		t.Fatalf("父 ctx 丢失 account_id: %v", m)
	}
	if _, ok := m["task_id"]; ok {
		t.Fatalf("父 ctx 不应有 task_id: %v", m)
	}

	// trace_id 与通用字段同时生效
	buf.Reset()
	Info(WithTraceID(child, "tid"), "all")
	m = lastLine(t, buf)
	if m["trace_id"] != "tid" || m["account_id"] != "u1" {
		t.Fatalf("trace+attrs 未同时生效: %v", m)
	}
}

// TestInjectionPriority 注入优先级:SetHandler 生效 → SetLogger(nil) 还原默认 → SetConfig 不覆盖 external
func TestInjectionPriority(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	Info(context.Background(), "via external")
	if !strings.Contains(buf.String(), "via external") {
		t.Fatal("SetHandler 注入未生效")
	}

	// SetConfig 重建默认,但不应覆盖已注入的 external
	SetConfig(Config{Level: "info", Format: "json", Output: "console"})
	buf.Reset()
	Info(context.Background(), "still external")
	if !strings.Contains(buf.String(), "still external") {
		t.Fatal("SetConfig 不应覆盖已注入的 external logger")
	}

	// SetLogger(nil) 还原默认(输出走 console,buf 不再增长)
	SetLogger(nil)
	buf.Reset()
	Info(context.Background(), "to default")
	if buf.Len() != 0 {
		t.Fatal("SetLogger(nil) 后不应再写入注入的 buf")
	}
}

// TestEnsureTraceID 兜底生成:16 位 hex;已有则幂等;NewTraceID 不重复
func TestEnsureTraceID(t *testing.T) {
	ctx := EnsureTraceID(context.Background())
	tid := TraceIDFromContext(ctx)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(tid) {
		t.Fatalf("生成的 trace_id 不是 16 位 hex: %q", tid)
	}

	// 幂等:已有 trace 的 ctx 原样返回
	ctx2 := EnsureTraceID(ctx)
	if TraceIDFromContext(ctx2) != tid {
		t.Fatalf("EnsureTraceID 不幂等: %q != %q", TraceIDFromContext(ctx2), tid)
	}

	// 显式注入优先
	pre := WithTraceID(context.Background(), "caller-set")
	if got := TraceIDFromContext(EnsureTraceID(pre)); got != "caller-set" {
		t.Fatalf("调用方 trace_id 被覆盖: %q", got)
	}

	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id := NewTraceID()
		if _, dup := seen[id]; dup {
			t.Fatalf("NewTraceID 重复: %q", id)
		}
		seen[id] = struct{}{}
	}
}

// TestSourceLocation AddSource 输出应指向业务调用点(本测试文件),而非 logger 包内部
func TestSourceLocation(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{AddSource: true})

	Info(context.Background(), "where am i")
	m := lastLine(t, buf)
	src, _ := m["source"].(map[string]any)
	if src == nil {
		t.Fatalf("缺少 source 字段: %v", m)
	}
	file, _ := src["file"].(string)
	if !strings.HasSuffix(file, "logger_test.go") {
		t.Fatalf("source.file = %q, want *logger_test.go(callers skip 偏移错误)", file)
	}
}

// TestErrNilSafe Err(nil)/NamedErr(k, nil) 不产生任何键
func TestErrNilSafe(t *testing.T) {
	buf := captureJSON(t, nil)

	Info(context.Background(), "nil errs", Err(nil), NamedErr("classify_err", nil))
	m := lastLine(t, buf)
	if _, ok := m["error"]; ok {
		t.Fatalf("Err(nil) 不应输出 error 键: %v", m)
	}
	if _, ok := m["classify_err"]; ok {
		t.Fatalf("NamedErr(k, nil) 不应输出键: %v", m)
	}

	// 非 nil 正常输出
	buf.Reset()
	Info(context.Background(), "real err", Err(context.DeadlineExceeded))
	if m := lastLine(t, buf); m["error"] != context.DeadlineExceeded.Error() {
		t.Fatalf("Err 输出错误: %v", m["error"])
	}
}

// TestConcurrentSetAndLog -race 下并发混跑 SetLogger/SetHandler/Info(atomic.Pointer 正确性)
func TestConcurrentSetAndLog(t *testing.T) {
	t.Cleanup(func() { SetLogger(nil) })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				SetHandler(slog.NewJSONHandler(&bytes.Buffer{}, nil))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				SetLogger(nil)
			}
		}()
		go func() {
			defer wg.Done()
			ctx := WithTraceID(context.Background(), "race")
			for j := 0; j < 100; j++ {
				Info(ctx, "concurrent", slog.Int("j", j))
			}
		}()
	}
	wg.Wait()
}
