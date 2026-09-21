package interceptor

import (
	"log/slog"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

type tracingInterceptor struct{}

// NewTracingInterceptor 为其内层的整段 HTTP 链建立一个轻量 span。
// 输入：无；请求 method/url/ctx 在 Intercept 时从 Chain 获取。
// 返回：可放在链最外层的 Interceptor；不引入 OpenTelemetry，也不实现 SideChannel
// （它会派生并回写 Request.Ctx，不是只读快照）。
func NewTracingInterceptor() httpx.Interceptor { return &tracingInterceptor{} }

// Intercept 为其内层打 httpx.request span。
// 输入 ch：用 StartSpan 的 ctx 覆盖 Request.Ctx，Proceed 返回后恢复父 ctx。
// 返回：内层 Proceed 的 resp/err 原样穿透；不触发缓存、不关资源。
func (i *tracingInterceptor) Intercept(ch *httpx.Chain) (resp *httpx.Response, err error) {
	req := ch.Request()
	parentCtx := req.Ctx
	spanCtx, end := logger.StartSpan(parentCtx, httpx.SpanHTTPRequest,
		slog.String(httpx.LogFieldMethod, req.Method),
		slog.String(httpx.LogFieldURL, req.FullURL))
	req.Ctx = spanCtx

	defer func() {
		req.Ctx = parentCtx
		attrs := make([]slog.Attr, 0, 2)
		if resp != nil {
			attrs = append(attrs, slog.Int(httpx.LogFieldStatus, resp.StatusCode))
		}
		if err != nil {
			attrs = append(attrs, logger.Err(err))
		}
		end(attrs...)
	}()

	return ch.Proceed()
}
