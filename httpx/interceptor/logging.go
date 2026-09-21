package interceptor

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

type loggingInterceptor struct{}

// NewLoggingInterceptor 打一条 event=http.transaction 结构化日志。
// 给默认链：默认全量字段；LogSummaryOnly 时 2xx/3xx 只打 6 个摘要，4xx/5xx 仍全量。
// 输入：无。返回可放入链的 Interceptor。
func NewLoggingInterceptor() httpx.Interceptor { return &loggingInterceptor{} }

// Intercept 先 Proceed，成功后再打日志。
// 输入 ch：不改 Request / Client；读 Options 与 LogBodyLimit。
// 返回：内层错误原样穿透（不打成功交易日志）；resp 成功则打完后原样返回。
func (i *loggingInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	start := time.Now()
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}

	durMs := time.Since(start).Milliseconds()
	c := ch.Client()
	slowMS := c.SlowMS()
	slow := slowMS > 0 && durMs >= int64(slowMS)

	req := ch.Request()
	limit := c.LogBodyLimit()

	if resp.StatusCode >= 400 || !c.Options().LogSummaryOnly {
		attrs := []slog.Attr{
			slog.String(httpx.LogFieldMethod, req.Method),
			slog.String(httpx.LogFieldURL, req.FullURL),
			slog.String(httpx.LogFieldProxy, c.Options().ProxyURL),
			slog.String(httpx.LogFieldExitIP, c.Options().ExitIP),
			slog.String(httpx.LogFieldASN, c.Options().ASN),
			slog.Any(httpx.LogFieldReqHeaders, req.ReqHeaders),
			slog.Any(httpx.LogFieldReqParams, req.Params),
			slog.String(httpx.LogFieldReqBody, string(httpx.TruncateBodyForLog(req.Body, limit))),
			slog.Int(httpx.LogFieldReqBodyLen, len(req.Body)),
			slog.Int(httpx.LogFieldStatus, resp.StatusCode),
			slog.String(httpx.LogFieldProto, resp.Proto),
			slog.Int(httpx.LogFieldProtoMajor, resp.ProtoMajor),
			slog.Any(httpx.LogFieldRespHeaders, resp.Header),
			slog.String(httpx.LogFieldRespBody, string(httpx.TruncateBodyForLog(resp.Body, limit))),
			slog.Int(httpx.LogFieldRespBodyLen, len(resp.Body)),
			slog.Int64(httpx.LogFieldDurationMS, durMs),
		}
		emit(req.Ctx, slow, attrs)
	} else {
		attrs := []slog.Attr{
			slog.String(httpx.LogFieldMethod, req.Method),
			slog.String(httpx.LogFieldURL, req.FullURL),
			slog.Int(httpx.LogFieldStatus, resp.StatusCode),
			slog.Int(httpx.LogFieldReqBodyLen, len(req.Body)),
			slog.Int(httpx.LogFieldRespBodyLen, len(resp.Body)),
			slog.Int64(httpx.LogFieldDurationMS, durMs),
		}
		emit(req.Ctx, slow, attrs)
	}
	return resp, nil
}

func emit(ctx context.Context, slow bool, attrs []slog.Attr) {
	if slow {
		logger.WarnEvent(ctx, httpx.EventHTTPTransaction, append(slices.Clone(attrs), slog.Bool(httpx.LogFieldSlow, true))...)
		return
	}
	logger.InfoEvent(ctx, httpx.EventHTTPTransaction, attrs...)
}
