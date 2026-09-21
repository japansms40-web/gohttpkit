package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

type changingEvent struct{ calls int }

func (e *changingEvent) Name() string {
	e.calls++
	return fmt.Sprintf("demo.%d", e.calls)
}

func TestInfoEvent_Name只求值一次(t *testing.T) {
	buf := captureJSON(t, nil)
	e := &changingEvent{}
	InfoEvent(context.Background(), e)
	got := lastLine(t, buf)
	t.Logf("calls=%d msg=%v event=%v", e.calls, got["msg"], got["event"])
	if e.calls != 1 || got["msg"] != "demo.1" || got["event"] != "demo.1" {
		t.Fatalf("calls=%d msg=%v event=%v", e.calls, got["msg"], got["event"])
	}
}

func TestInfoEvent_nil记录空名(t *testing.T) {
	buf := captureJSON(t, nil)
	var e Event
	InfoEvent(context.Background(), e)
	got := lastLine(t, buf)
	t.Logf("nil Event → msg=%v event=%v", got["msg"], got["event"])
	if got["msg"] != "" || got["event"] != "" {
		t.Fatalf("msg=%v event=%v", got["msg"], got["event"])
	}
}

func TestInfoEvent_不改调用方容量区域(t *testing.T) {
	buf := captureJSON(t, nil)
	storage := make([]slog.Attr, 2)
	storage[0] = slog.String("first", "1")
	storage[1] = slog.String("sentinel", "keep")
	InfoEvent(context.Background(), NewEvent("demo.done"), storage[:1]...)
	_ = lastLine(t, buf)
	t.Logf("storage[1]=%v", storage[1])
	if storage[1].Key != "sentinel" || storage[1].Value.String() != "keep" {
		t.Fatalf("调用方底层数组被覆盖: %v", storage[1])
	}
}

func TestNewEvent_空名按字面保留(t *testing.T) {
	got := NewEvent("").Name()
	t.Logf("NewEvent(\"\").Name()=%q", got)
	if got != "" {
		t.Fatalf("Name()=%q, want empty", got)
	}
}

func TestAttr_nil输出空事件(t *testing.T) {
	attr := Attr(nil)
	t.Logf("Attr(nil) → key=%q value=%q", attr.Key, attr.Value.String())
	if attr.Key != "event" || attr.Value.String() != "" {
		t.Fatalf("Attr(nil)=%v", attr)
	}
}

func TestEventAttr_不改普通日志msg(t *testing.T) {
	buf := captureJSON(t, nil)
	Info(context.Background(), "给人看的文案", EventAttr("demo.finished"))
	got := lastLine(t, buf)
	t.Logf("msg=%v event=%v", got["msg"], got["event"])
	if got["msg"] != "给人看的文案" || got["event"] != "demo.finished" {
		t.Fatalf("msg=%v event=%v", got["msg"], got["event"])
	}
}

func TestWarnEvent_级别与名称稳定(t *testing.T) {
	buf := captureJSON(t, nil)
	WarnEvent(context.Background(), NewEvent("demo.warn"))
	got := lastLine(t, buf)
	t.Logf("level=%v msg=%v event=%v", got["level"], got["msg"], got["event"])
	if got["level"] != "WARN" || got["msg"] != "demo.warn" || got["event"] != "demo.warn" {
		t.Fatalf("level=%v msg=%v event=%v", got["level"], got["msg"], got["event"])
	}
}

func TestInfoEvent_AddSource指向调用点(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{AddSource: true})
	InfoEvent(context.Background(), NewEvent("demo.source"))
	got := lastLine(t, buf)
	src, _ := got["source"].(map[string]any)
	file, _ := src["file"].(string)
	t.Logf("source.file=%q", file)
	if !strings.HasSuffix(file, "event_test.go") {
		t.Fatalf("source.file=%q, want *event_test.go", file)
	}
}
