package interceptor

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

type retryInterceptor struct{}

const sendRequestErrFmt = "failed to send request: %w"

// computeBackoff 第 attempt 次重试的退避时长：base * 2^attempt，封顶 limit（limit<=0 表示不封顶）。
// 输入 base：退避基数（已由 normalizeRetry 保证 >0）；limit：单次封顶；attempt：从 0 起的重试序号。
// 返回：base<<attempt，超过 limit 时钳到 limit；base>0 时结果恒 >0。
// 逐次翻倍而不是一步 `base * (1 << attempt)`：后者在大 attempt 下会整型溢出成负数或 0，
// 让 `backoff > limit` 判断失效、把 MaxBackoff 封顶绕过并触发立即重试（重试风暴）。
// 翻倍前先判断是否会越过 limit 或越过 int64 上限，越过就钳住，绝不溢出。
// 形参用 limit 不用 max：避免遮蔽 Go 1.21 内建 max（revive redefines-builtin-id）。
func computeBackoff(base, limit time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	backoff := base
	for i := 0; i < attempt; i++ {
		if limit > 0 && backoff >= limit {
			return limit
		}
		if backoff > math.MaxInt64/2 { // 再翻倍会溢出 int64
			if limit > 0 {
				return limit
			}
			return backoff // 不封顶：停在溢出前的安全上限
		}
		backoff *= 2
	}
	if limit > 0 && backoff > limit {
		return limit
	}
	return backoff
}

// NewRetryInterceptor 网络层重试：包住 bridge + 终端，每次重试重新构建请求与请求头。
// 给默认链：策略只认 New 时已归一化的 Options.Retry（经 Client.RetryPolicy()），不读环境变量。
// 输入：无。返回可放入链的 Interceptor。
// 只对 *httpx.TransportError 做可重试判断；bridge 产生的错误（构建请求失败、构头失败）
// 原样穿透立即返回 —— 那是代码/配置错误，重试一万次也是同样结果。
func NewRetryInterceptor() httpx.Interceptor { return &retryInterceptor{} }

// Intercept 按 RetryPolicy 循环调用 Proceed。
// 输入 ch：会改写 Request.Attempt；成功或不可重试时立即返回，不改 Client。
// 返回：内层成功 → 原样 resp；非 *httpx.TransportError → 原样穿透；
// 不可重试网络错、或失败正由调用方 ctx 结束引起 → fmt.Errorf("failed to send request: %w", err)；
// 用尽次数、或可重试失败后在退避中 ctx 取消 → *errors.RetryableError。
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

		// 调用方 ctx 已结束且本次失败正由它引起：不是网络抖动，不重试、不打 retry 告警，
		// 原样交还（调用方可 errors.Is 判 Canceled / DeadlineExceeded）。
		// http.Client.Timeout 的错误链同样含 DeadlineExceeded，但那时 req.Ctx 仍存活，照常重试。
		if ctxErr := req.Ctx.Err(); ctxErr != nil && stderrors.Is(doErr, ctxErr) {
			return nil, fmt.Errorf(sendRequestErrFmt, doErr)
		}

		if !policy.IsRetryable(doErr) {
			return nil, fmt.Errorf(sendRequestErrFmt, doErr)
		}

		if attempt == policy.MaxRetries {
			return nil, &errors.RetryableError{Err: doErr, Attempts: attempt + 1, LastError: lastErr}
		}

		backoff := computeBackoff(policy.BaseBackoff, policy.MaxBackoff, attempt)

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

	// coverage:ignore  normalizeRetry 保证 MaxRetries>=0，循环必从 return 退出
	return nil, fmt.Errorf(sendRequestErrFmt, lastErr)
}
