package interceptor

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

type retryInterceptor struct{}

const sendRequestErrFmt = "failed to send request: %w"

// NewRetryInterceptor 网络层重试：包住 bridge + 终端，每次重试重新构建请求与请求头。
// 给默认链：策略只认 New 时已归一化的 Options.Retry（经 Client.RetryPolicy()），不读环境变量。
// 输入：无。返回可放入链的 Interceptor。
// 只对 *httpx.TransportError 做可重试判断；bridge 产生的错误（构建请求失败、构头失败）
// 原样穿透立即返回 —— 那是代码/配置错误，重试一万次也是同样结果。
func NewRetryInterceptor() httpx.Interceptor { return &retryInterceptor{} }

// Intercept 按 RetryPolicy 循环调用 Proceed。
// 输入 ch：会改写 Request.Attempt；成功或不可重试时立即返回，不改 Client。
// 返回：内层成功 → 原样 resp；非 *httpx.TransportError → 原样穿透；
// 不可重试网络错 → fmt.Errorf("failed to send request: %w", err)；
// 用尽次数或 ctx 取消 → *errors.RetryableError。
func (i *retryInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	policy := ch.Client().RetryPolicy()
	req := ch.Request()

	var lastErr error
	retryStart := time.Now()

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		req.Attempt = attempt

		resp, err := ch.Proceed()
		if err == nil {
			if attempt > 0 {
				logger.InfoEvent(req.Ctx, httpx.EventHTTPRetrySucceeded,
					slog.String(httpx.LogFieldMethod, req.Method),
					slog.String(httpx.LogFieldURL, req.FullURL),
					slog.Int(httpx.LogFieldAttempt, attempt+1),
					logger.DurationMs(time.Since(retryStart)),
				)
			}
			return resp, nil
		}

		var terr *httpx.TransportError
		if !stderrors.As(err, &terr) {
			return nil, err
		}
		doErr := terr.Err
		lastErr = doErr

		if !policy.IsRetryable(doErr) {
			return nil, fmt.Errorf(sendRequestErrFmt, doErr)
		}

		if attempt == policy.MaxRetries {
			return nil, &errors.RetryableError{Err: doErr, Attempts: attempt + 1, LastError: lastErr}
		}

		backoff := policy.BaseBackoff * (1 << uint(attempt))
		if policy.MaxBackoff > 0 && backoff > policy.MaxBackoff {
			backoff = policy.MaxBackoff
		}

		logger.WarnEvent(req.Ctx, httpx.EventHTTPRetry,
			slog.String(httpx.LogFieldMethod, req.Method),
			slog.String(httpx.LogFieldURL, req.FullURL),
			slog.Any(httpx.LogFieldReqHeaders, req.ReqHeaders),
			slog.Any(httpx.LogFieldReqParams, req.Params),
			slog.String(httpx.LogFieldReqBody, string(httpx.TruncateBodyForLog(req.Body, ch.Client().LogBodyLimit()))),
			slog.Int(httpx.LogFieldReqBodyLen, len(req.Body)),
			slog.Int(httpx.LogFieldAttempt, attempt+1),
			slog.Int(httpx.LogFieldMaxRetries, policy.MaxRetries),
			slog.Duration(httpx.LogFieldBackoff, backoff),
			slog.String(httpx.LogFieldErrType, fmt.Sprintf("%T", doErr)),
			logger.Err(doErr),
		)

		select {
		case <-req.Ctx.Done():
			return nil, &errors.RetryableError{Err: req.Ctx.Err(), Attempts: attempt + 1, LastError: lastErr}
		case <-time.After(backoff):
		}
	}

	return nil, fmt.Errorf(sendRequestErrFmt, lastErr)
}
