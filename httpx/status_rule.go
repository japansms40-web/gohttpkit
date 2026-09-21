package httpx

import (
	"strings"

	"github.com/japansms40-web/gohttpkit/errors"
)

// StatusRule 决定一个非 2xx 响应该被翻译成什么错误。
//
// 返回 nil     → 放行：响应原样继续向外流（调用方自己解析体 + 读状态码）
// 返回非 nil   → 整次调用以该 error 失败
type StatusRule func(status int, body []byte) error

// DefaultStatusRule 默认规则：非 2xx 且响应体为空 → *errors.HTTPStatusError。
// 给 NewStatusSemanticsInterceptor：响应体非空则放行（4xx 常承载业务体）。
// 输入 status：HTTP 状态码；body 可为 nil/空。
// 返回：有体 → nil；空体 → *errors.HTTPStatusError{StatusCode, Body}。
// 例：DefaultStatusRule(404, nil) → HTTPStatusError；DefaultStatusRule(400, []byte(`{}`)) → nil。
func DefaultStatusRule(status int, body []byte) error {
	if len(body) > 0 {
		return nil
	}
	return &errors.HTTPStatusError{StatusCode: status, Body: body}
}

// RetryableTextRule 生成一条规则：非 2xx 体命中关键词或状态码在名单里 → *errors.RetryableError。
// 给限流文案不是 429 的接口：命中后调用方可换代理续跑；否则回落 DefaultStatusRule。
// 输入 keywords：空串条目被忽略；retryStatuses 可空。
// 返回：闭包 StatusRule，不改入参切片。
// 例：RetryableTextRule([]string{"rate limit"}, 503)(503, nil) → *errors.RetryableError。
func RetryableTextRule(keywords []string, retryStatuses ...int) StatusRule {
	statusSet := make(map[int]struct{}, len(retryStatuses))
	for _, s := range retryStatuses {
		statusSet[s] = struct{}{}
	}
	return func(status int, body []byte) error {
		hit := false
		text := string(body)
		for _, kw := range keywords {
			if kw != "" && strings.Contains(text, kw) {
				hit = true
				break
			}
		}
		if _, ok := statusSet[status]; ok {
			hit = true
		}
		if hit {
			cause := &errors.HTTPStatusError{StatusCode: status, Body: body}
			return &errors.RetryableError{Err: cause, Attempts: 1, LastError: cause}
		}
		return DefaultStatusRule(status, body)
	}
}
