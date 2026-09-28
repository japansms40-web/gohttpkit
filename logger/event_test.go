package logger

import (
	"fmt"
	"log/slog"
	"reflect"
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
	InfoEvent(t.Context(), e)
	got := lastLine(t, buf)
	t.Logf("calls=%d msg=%v event=%v", e.calls, got["msg"], got["event"])
	if e.calls != 1 || got["msg"] != "demo.1" || got["event"] != "demo.1" {
		t.Fatalf("calls=%d msg=%v event=%v", e.calls, got["msg"], got["event"])
	}
}

func TestInfoEvent_nil记录空名(t *testing.T) {
	buf := captureJSON(t, nil)
	var e Event
	InfoEvent(t.Context(), e)
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
	InfoEvent(t.Context(), NewEvent("demo.done"), storage[:1]...)
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
	Info(t.Context(), "给人看的文案", EventAttr("demo.finished"))
	got := lastLine(t, buf)
	t.Logf("msg=%v event=%v", got["msg"], got["event"])
	if got["msg"] != "给人看的文案" || got["event"] != "demo.finished" {
		t.Fatalf("msg=%v event=%v", got["msg"], got["event"])
	}
}

func TestWarnEvent_级别与名称稳定(t *testing.T) {
	buf := captureJSON(t, nil)
	WarnEvent(t.Context(), NewEvent("demo.warn"))
	got := lastLine(t, buf)
	t.Logf("level=%v msg=%v event=%v", got["level"], got["msg"], got["event"])
	if got["level"] != "WARN" || got["msg"] != "demo.warn" || got["event"] != "demo.warn" {
		t.Fatalf("level=%v msg=%v event=%v", got["level"], got["msg"], got["event"])
	}
}

func TestInfoEvent_AddSource指向调用点(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{AddSource: true})
	InfoEvent(t.Context(), NewEvent("demo.source"))
	got := lastLine(t, buf)
	src, _ := got["source"].(map[string]any)
	file, _ := src["file"].(string)
	t.Logf("source.file=%q", file)
	if !strings.HasSuffix(file, "event_test.go") {
		t.Fatalf("source.file=%q, want *event_test.go", file)
	}
}

type accountEvent string

func (e accountEvent) Name() string { return string(e) }

func TestEvent_外部包可实现(t *testing.T) {
	var e Event = accountEvent("account.login.failed")
	attr := Attr(e)
	t.Logf("name=%q attr=%v", e.Name(), attr)
	if e.Name() != "account.login.failed" || attr.Key != "event" ||
		attr.Value.String() != e.Name() {
		t.Fatalf("name=%q attr=%v", e.Name(), attr)
	}
	// 本文件是内部测试包：再用反射锁住「接口方法全部导出」，外部包才实现得了。
	typ := reflect.TypeFor[Event]()
	for i := range typ.NumMethod() {
		if m := typ.Method(i); !m.IsExported() {
			t.Fatalf("Event 方法 %s 未导出，外部包无法实现", m.Name)
		}
	}
}
