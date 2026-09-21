package httpx

import "github.com/japansms40-web/gohttpkit/logger"

// events.go —— HTTP 领域的机器事件。独立成文件：事件契约与拦截器实现分开，
// 改机器名时只动这一处，测试与打点处不再各自写字面量。

var (
	// EventHTTPTransaction 一次成功 HTTP 交易。接入方按 event=http.transaction 过滤；msg 与 Name 同值。
	EventHTTPTransaction = logger.NewEvent("http.transaction")
	// EventHTTPRetry 一次可重试失败后准备退避。接入方按 event=http.retry 过滤。
	EventHTTPRetry = logger.NewEvent("http.retry")
	// EventHTTPRetrySucceeded 重试之后才成功。接入方按 event=http.retry.succeeded 确认重试生效。
	EventHTTPRetrySucceeded = logger.NewEvent("http.retry.succeeded")
)

const (
	// SpanHTTPRequest 是默认链最外层 tracing 拦截器的 span_name。
	// StartSpan 打出的 span.start / span.end 用这个名字；自组链未加 NewTracingInterceptor 时不会出现。
	SpanHTTPRequest = "httpx.request"
)
