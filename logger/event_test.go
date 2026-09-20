package logger

import (
	"context"
	"testing"
)

func TestEvent_字段名与值稳定(t *testing.T) {
	a := Event("http.retry")
	t.Logf("Event(http.retry) → key=%q value=%q", a.Key, a.Value.String())
	if a.Key != "event" || a.Value.String() != "http.retry" {
		t.Fatalf("Event = key=%q value=%q", a.Key, a.Value.String())
	}
}

func TestEvent_空串按字面保留(t *testing.T) {
	a := Event("")
	t.Logf("Event(\"\") → key=%q value=%q", a.Key, a.Value.String())
	if a.Key != "event" || a.Value.String() != "" {
		t.Fatalf("空串应原样保留, got key=%q value=%q", a.Key, a.Value.String())
	}
}

func TestEvent_常量可直接当map的key(t *testing.T) {
	m := map[string]string{EventSpanStart: EventSpanStart, EventSpanEnd: EventSpanEnd}
	t.Logf("map=%v", m)
	if m[EventSpanStart] != "span.start" || m[EventSpanEnd] != "span.end" {
		t.Fatalf("event 常量必须是无类型字符串: %v", m)
	}
}

func TestEvent_不改msg语义(t *testing.T) {
	buf := captureJSON(t, nil)
	Info(context.Background(), "给人看的文案", Event("demo.finished"))
	m := lastLine(t, buf)
	t.Logf("msg=%v event=%v", m["msg"], m["event"])
	if m["msg"] != "给人看的文案" || m["event"] != "demo.finished" {
		t.Fatalf("msg/event 契约错误: %v", m)
	}
}
