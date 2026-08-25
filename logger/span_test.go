package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// allLines 解析 buf 全部 JSON 日志行
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

// TestStartSpanEmitsStartAndEnd StartSpan 产出 span.start/span.end,且带 span_name、span_id、duration_ms
// 角度: #6 契约不变式 —— span 边界事件必须成对且自带耗时
func TestStartSpanEmitsStartAndEnd(t *testing.T) {
	buf := captureJSON(t, nil)

	ctx, end := StartSpan(context.Background(), "demo.op")
	end(slog.String("result", "ok"))

	lines := allLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("应产出 span.start + span.end 两条,实际 %d 条: %s", len(lines), buf.String())
	}
	start, fin := lines[0], lines[1]

	if start["msg"] != "span.start" || fin["msg"] != "span.end" {
		t.Fatalf("消息名错误: start=%v end=%v", start["msg"], fin["msg"])
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
	// 上下文未带 trace_id 时,StartSpan 应兜底生成
	if start["trace_id"] == nil || start["trace_id"] == "" {
		t.Errorf("StartSpan 应确保 trace_id: %v", start)
	}
	_ = ctx
}

// TestSpanNesting 子 span 的 parent_span_id 等于父 span 的 span_id;trace_id 全链一致
// 角度: #6 契约不变式 —— 嵌套关系可还原调用树
func TestSpanNesting(t *testing.T) {
	buf := captureJSON(t, nil)

	parentCtx, endParent := StartSpan(WithTraceID(context.Background(), "tid-1"), "parent")
	childCtx, endChild := StartSpan(parentCtx, "child")
	endChild()
	endParent()
	_ = childCtx

	lines := allLines(t, buf)
	// 顺序: parent.start, child.start, child.end, parent.end
	parentStart := lines[0]
	childStart := lines[1]

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

// TestSpanCtxAutoCarriesSpanID span 内的普通 logger.Info 自动带当前 span_id
// 角度: #5 副作用 —— ctx 自动注入对普通日志同样生效
func TestSpanCtxAutoCarriesSpanID(t *testing.T) {
	buf := captureJSON(t, nil)

	ctx, end := StartSpan(context.Background(), "op")
	Info(ctx, "inside span work")
	end()

	lines := allLines(t, buf)
	spanID := lines[0]["span_id"]
	mid := lines[1] // "inside span work"
	if mid["msg"] != "inside span work" {
		t.Fatalf("中间日志顺序错误: %v", mid)
	}
	if mid["span_id"] != spanID || spanID == "" {
		t.Errorf("span 内普通日志应自动带 span_id %v,实际 %v", spanID, mid["span_id"])
	}
}

// TestDurationMs DurationMs 统一字段名 duration_ms,单位毫秒
// 角度: #6 契约不变式 —— 全项目耗时字段同名同单位
func TestDurationMs(t *testing.T) {
	a := DurationMs(1500 * 1e6) // 1500ms = 1.5s
	if a.Key != "duration_ms" {
		t.Errorf("字段名应为 duration_ms,实际 %s", a.Key)
	}
	if got := a.Value.Int64(); got != 1500 {
		t.Errorf("1.5s 应为 1500ms,实际 %d", got)
	}
}
