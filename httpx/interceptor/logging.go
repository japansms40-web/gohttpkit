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
			slog.String("method", req.Method),
			slog.String("url", req.FullURL),
			slog.String("proxy", c.Options().ProxyURL),
			slog.String("exit_ip", c.Options().ExitIP),
			slog.String("asn", c.Options().ASN),
			slog.Any("req_headers", req.ReqHeaders),
			slog.Any("req_params", req.Params),
			slog.String("req_body", string(httpx.TruncateBodyForLog(req.Body, limit))),
			slog.Int("req_body_len", len(req.Body)),
			slog.Int("status", resp.StatusCode),
			slog.String("proto", resp.Proto),
			slog.Int("proto_major", resp.ProtoMajor),
			slog.Any("resp_headers", resp.Header),
			slog.String("resp_body", string(httpx.TruncateBodyForLog(resp.Body, limit))),
			slog.Int("resp_body_len", len(resp.Body)),
			slog.Int64("duration_ms", durMs),
		}
		emit(req.Ctx, slow, attrs)
	} else {
		attrs := []slog.Attr{
			slog.String("method", req.Method),
			slog.String("url", req.FullURL),
			slog.Int("status", resp.StatusCode),
			slog.Int("req_body_len", len(req.Body)),
			slog.Int("resp_body_len", len(resp.Body)),
			slog.Int64("duration_ms", durMs),
		}
		emit(req.Ctx, slow, attrs)
	}
	return resp, nil
}

func emit(ctx context.Context, slow bool, attrs []slog.Attr) {
	if slow {
		logger.WarnEvent(ctx, httpx.EventHTTPTransaction, append(slices.Clone(attrs), slog.Bool("slow", true))...)
		return
	}
	logger.InfoEvent(ctx, httpx.EventHTTPTransaction, attrs...)
}
