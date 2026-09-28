package interceptor_test

// tracing_test.go —— NewTracingInterceptor 的单元契约：内层拿到新 span、返回后恢复父 ctx、
// span.end 按结果带 status / error、内层 resp/err 原样穿透。
// out-of-scope：并发（拦截器无共享可变状态，每次 Intercept 只改本次 Request）。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/logger"
)

// spanLogs 从 JSON 日志里挑出 span_name=httpx.request 的记录，按 event 分组。
func spanLogs(t *testing.T, raw string) map[string][]map[string]any {
	t.Helper()
	out := make(map[string][]map[string]any)
	for line := range strings.SplitSeq(raw, "\n") {
		var rec map[string]any
		if strings.TrimSpace(line) == "" || json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec[logger.FieldSpanName] != httpx.SpanHTTPRequest {
			continue
		}
		ev, _ := rec["event"].(string)
		out[ev] = append(out[ev], rec)
	}
	return out
}

// tracingClient 装配 [outer?] + tracing + inner 的最小链；inner 充当终端。
func tracingClient(t *testing.T, outer httpx.Interceptor, inner httpx.InterceptorFunc) *httpx.Client {
	t.Helper()
	chain := httpx.Interceptors{}
	if outer != nil {
		chain = append(chain, outer)
	}
	chain = append(chain, interceptor.NewTracingInterceptor(), inner)
	return newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://trace.example"},
		Interceptors: chain,
	})
}

func TestTracing_内层拿到新span且返回后恢复父ctx(t *testing.T) {
	var before, after, inner context.Context
	outer := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		before = ch.Request().Ctx
		resp, err := ch.Proceed()
		after = ch.Request().Ctx
		return resp, err
	})
	c := tracingClient(t, outer, func(ch *httpx.Chain) (*httpx.Response, error) {
		inner = ch.Request().Ctx
		return &httpx.Response{StatusCode: http.StatusOK, Header: http.Header{}}, nil
	})
	if _, err := c.Get(logger.WithTraceID(t.Context(), "tid-trace"), "/x", nil); err != nil {
		t.Fatal(err)
	}
	innerSpan := logger.SpanIDFromContext(inner)
	t.Logf("before span=%q inner span=%q after span=%q inner trace=%q",
		logger.SpanIDFromContext(before), innerSpan, logger.SpanIDFromContext(after), logger.TraceIDFromContext(inner))
	if innerSpan == "" {
		t.Fatal("内层 Request.Ctx 应带 tracing 新开的 span_id")
	}
	if logger.TraceIDFromContext(inner) != "tid-trace" {
		t.Fatalf("span 应沿用调用方 trace_id，得到 %q", logger.TraceIDFromContext(inner))
	}
	if after != before {
		t.Fatal("Proceed 返回后 Request.Ctx 应恢复为进入 tracing 前的父 ctx")
	}
}

func TestTracing_成功时spanEnd带status且start带method与url(t *testing.T) {
	buf := captureInterceptorLogs(t)
	c := tracingClient(t, nil, func(*httpx.Chain) (*httpx.Response, error) {
		return &httpx.Response{StatusCode: http.StatusCreated, Header: http.Header{}}, nil
	})
	if _, err := c.Get(t.Context(), "/ok", nil); err != nil {
		t.Fatal(err)
	}
	logs := spanLogs(t, buf.String())
	t.Logf("span logs=%v", logs)
	starts, ends := logs[logger.EventSpanStart.Name()], logs[logger.EventSpanEnd.Name()]
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("应各打一条 span.start / span.end，得到 start=%d end=%d", len(starts), len(ends))
	}
	if starts[0][httpx.LogFieldMethod] != http.MethodGet || starts[0][httpx.LogFieldURL] != "https://trace.example/ok" {
		t.Fatalf("span.start 应带 method/url，得到 %v", starts[0])
	}
	if ends[0][httpx.LogFieldStatus] != float64(http.StatusCreated) {
		t.Fatalf("span.end 应带 status=201，得到 %v", ends[0])
	}
	if _, ok := ends[0][logger.FieldError]; ok {
		t.Fatalf("成功时 span.end 不应带 error，得到 %v", ends[0])
	}
}

func TestTracing_内层错误原样穿透且spanEnd只带error(t *testing.T) {
	buf := captureInterceptorLogs(t)
	sentinel := errors.New("inner boom")
	c := tracingClient(t, nil, func(*httpx.Chain) (*httpx.Response, error) {
		return nil, sentinel
	})
	_, err := c.Get(t.Context(), "/fail", nil)
	t.Logf("err=%v", err)
	if !errors.Is(err, sentinel) {
		t.Fatalf("内层错误应原样穿透，得到 %v", err)
	}
	ends := spanLogs(t, buf.String())[logger.EventSpanEnd.Name()]
	if len(ends) != 1 {
		t.Fatalf("失败也应打一条 span.end，得到 %d", len(ends))
	}
	if !strings.Contains(ends[0][logger.FieldError].(string), "inner boom") {
		t.Fatalf("span.end 应带内层错误文案，得到 %v", ends[0])
	}
	if _, ok := ends[0][httpx.LogFieldStatus]; ok {
		t.Fatalf("resp 为 nil 时 span.end 不应带 status，得到 %v", ends[0])
	}
}

func TestTracing_不是旁路也不是终端(t *testing.T) {
	it := interceptor.NewTracingInterceptor()
	t.Logf("IsSideChannel=%v IsTerminal=%v", httpx.IsSideChannel(it), httpx.IsTerminal(it))
	if httpx.IsSideChannel(it) || httpx.IsTerminal(it) {
		t.Fatal("tracing 会回写 Request.Ctx 且调用 Proceed，不能标成旁路或终端")
	}
}
