package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"
)

func allLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("解析日志 JSON 失败: %v\n原文: %s", err, line)
		}
		out = append(out, m)
	}
	return out
}

func TestStartSpan_成对事件并带耗时(t *testing.T) {
	buf := captureJSON(t, nil)

	ctx, end := StartSpan(context.Background(), "demo.op")
	end(slog.String("result", "ok"))

	lines := allLines(t, buf)
	t.Logf("lines=%d start=%v end=%v", len(lines), lines[0], lines[1])
	if len(lines) != 2 {
		t.Fatalf("应产出 span.start + span.end 两条,实际 %d 条: %s", len(lines), buf.String())
	}
	start, fin := lines[0], lines[1]

	if start["msg"] != EventSpanStart.Name() || fin["msg"] != EventSpanEnd.Name() {
		t.Fatalf("span msg 契约错误: start=%v end=%v", start["msg"], fin["msg"])
	}
	if start["event"] != EventSpanStart.Name() || fin["event"] != EventSpanEnd.Name() {
		t.Fatalf("span event 契约错误: start=%v end=%v", start["event"], fin["event"])
	}
	if start["span_name"] != "demo.op" || fin["span_name"] != "demo.op" {
		t.Errorf("span_name 缺失或不一致: %v / %v", start["span_name"], fin["span_name"])
	}
	if start["span_id"] == nil || start["span_id"] == "" {
		t.Errorf("span.start 应自动带非空 span_id: %v", start)
	}
	if start["span_id"] != fin["span_id"] {
		t.Errorf("start/end 应属同一 span_id: %v vs %v", start["span_id"], fin["span_id"])
	}
	if _, ok := fin["duration_ms"]; !ok {
		t.Errorf("span.end 应带 duration_ms: %v", fin)
	}
	if fin["result"] != "ok" {
		t.Errorf("end() 追加的结果字段丢失: %v", fin)
	}
	if start["trace_id"] == nil || start["trace_id"] == "" {
		t.Errorf("StartSpan 应确保 trace_id: %v", start)
	}
	_ = ctx
}

func TestStartSpan_nilContext与空名(t *testing.T) {
	buf := captureJSON(t, nil)
	ctx, end := StartSpan(nil, "")
	end()
	lines := allLines(t, buf)
	t.Logf("nil/empty → start=%v", lines[0])
	if len(lines) != 2 {
		t.Fatalf("应仍产出两条,实际 %d", len(lines))
	}
	if lines[0]["span_name"] != "" {
		t.Errorf("空 name 应写出空串 span_name, got %v", lines[0]["span_name"])
	}
	if TraceIDFromContext(ctx) == "" || SpanIDFromContext(ctx) == "" {
		t.Fatal("nil ctx 也应带上 trace_id 与 span_id")
	}
}

func TestSpanNesting_parent与trace一致(t *testing.T) {
	buf := captureJSON(t, nil)

	parentCtx, endParent := StartSpan(WithTraceID(context.Background(), "tid-1"), "parent")
	childCtx, endChild := StartSpan(parentCtx, "child")
	endChild()
	endParent()
	_ = childCtx

	lines := allLines(t, buf)
	parentStart := lines[0]
	childStart := lines[1]
	t.Logf("parent=%v child=%v", parentStart["span_id"], childStart["span_id"])

	if parentStart["parent_span_id"] != nil {
		t.Errorf("根 span 不应有 parent_span_id: %v", parentStart["parent_span_id"])
	}
	if childStart["parent_span_id"] != parentStart["span_id"] {
		t.Errorf("子 span 的 parent_span_id(%v) 应等于父 span_id(%v)",
			childStart["parent_span_id"], parentStart["span_id"])
	}
	if childStart["span_id"] == parentStart["span_id"] {
		t.Errorf("父子 span_id 不应相同: %v", childStart["span_id"])
	}
	for _, m := range lines {
		if m["trace_id"] != "tid-1" {
			t.Errorf("全链 trace_id 应一致为 tid-1: %v", m)
		}
	}
}

func TestSpanCtx_普通Info自动带span_id(t *testing.T) {
	buf := captureJSON(t, nil)

	ctx, end := StartSpan(context.Background(), "op")
	Info(ctx, "inside span work")
	end()

	lines := allLines(t, buf)
	spanID := lines[0]["span_id"]
	mid := lines[1]
	t.Logf("span_id=%v mid=%v", spanID, mid)
	if mid["msg"] != "inside span work" {
		t.Fatalf("中间日志顺序错误: %v", mid)
	}
	if mid["span_id"] != spanID || spanID == "" {
		t.Errorf("span 内普通日志应自动带 span_id %v,实际 %v", spanID, mid["span_id"])
	}
}

func TestSpanIDFromContext_空与nil(t *testing.T) {
	if got := SpanIDFromContext(nil); got != "" {
		t.Fatalf("nil → %q", got)
	}
	if got := SpanIDFromContext(context.Background()); got != "" {
		t.Fatalf("Background → %q", got)
	}
	t.Logf("无 span 时为空串")
}

func TestNewSpanID_格式为8位hex(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}$`)
	id := NewSpanID()
	t.Logf("NewSpanID = %q", id)
	if !re.MatchString(id) {
		t.Fatalf("不是 8 位 hex: %q", id)
	}
}

func TestStartSpan_end可多次调用(t *testing.T) {
	buf := captureJSON(t, nil)
	_, end := StartSpan(context.Background(), "multi.end")
	end()
	end(slog.String("again", "yes"))
	lines := allLines(t, buf)
	t.Logf("lines=%d", len(lines))
	if len(lines) != 3 {
		t.Fatalf("start + 两次 end 应 3 条,实际 %d: %s", len(lines), buf.String())
	}
	if lines[1]["event"] != EventSpanEnd.Name() || lines[2]["event"] != EventSpanEnd.Name() {
		t.Fatalf("后两条都应是 %q: %v %v",
			EventSpanEnd.Name(), lines[1]["event"], lines[2]["event"])
	}
	if lines[2]["again"] != "yes" {
		t.Fatalf("第二次 end 的字段丢失: %v", lines[2])
	}
	if lines[1]["span_id"] != lines[2]["span_id"] || lines[1]["span_id"] != lines[0]["span_id"] {
		t.Fatalf("多次 end 应属同一 span: %v", lines)
	}
}

func TestStartSpan_start带调用方attrs(t *testing.T) {
	buf := captureJSON(t, nil)
	_, end := StartSpan(context.Background(), "with.attr", slog.String("method", "GET"))
	end()
	start := allLines(t, buf)[0]
	t.Logf("start=%v", start)
	if start["method"] != "GET" {
		t.Fatalf("start attrs 丢失 method: %v", start)
	}
	if start["event"] != EventSpanStart.Name() {
		t.Fatalf("event=%v", start["event"])
	}
}

func TestStartSpan_不改入参ctx(t *testing.T) {
	_ = captureJSON(t, nil)
	parent := context.Background()
	ctx, end := StartSpan(parent, "immutable")
	end()
	t.Logf("parent span=%q child span=%q", SpanIDFromContext(parent), SpanIDFromContext(ctx))
	if SpanIDFromContext(parent) != "" {
		t.Fatal("StartSpan 不应改写入参 ctx")
	}
	if SpanIDFromContext(ctx) == "" {
		t.Fatal("返回 ctx 应带 span_id")
	}
}

func TestNewSpanID_不重复(t *testing.T) {
	seen := make(map[string]struct{})
	re := regexp.MustCompile(`^[0-9a-f]{8}$`)
	for i := 0; i < 200; i++ {
		id := NewSpanID()
		if !re.MatchString(id) {
			t.Fatalf("NewSpanID = %q 不是 8 位 hex", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("NewSpanID 重复: %q", id)
		}
		seen[id] = struct{}{}
	}
	t.Logf("200 个 NewSpanID 无重复")
}

func TestDurationMs_字段名与毫秒(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want int64
	}{
		{"1.5秒", 1500 * time.Millisecond, 1500},
		{"零", 0, 0},
		{"负值", -2 * time.Millisecond, -2},
		{"不足一毫秒截断", 500 * time.Microsecond, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := DurationMs(c.in)
			t.Logf("DurationMs(%v) → key=%s val=%d", c.in, a.Key, a.Value.Int64())
			if a.Key != "duration_ms" {
				t.Errorf("字段名应为 duration_ms,实际 %s", a.Key)
			}
			if got := a.Value.Int64(); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
