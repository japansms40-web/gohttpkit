package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

type statusSemanticsInterceptor struct{ rule httpx.StatusRule }

// NewStatusSemanticsInterceptor 对非 2xx 响应套用 rule。
// 给想把空体 4xx/5xx 变成 error 的调用方；放链最外层附近，看最终对外的体。
// 输入 rule：nil 回落 DefaultStatusRule。返回可放入链的 Interceptor。
func NewStatusSemanticsInterceptor(rule httpx.StatusRule) httpx.Interceptor {
	if rule == nil {
		rule = httpx.DefaultStatusRule
	}
	return &statusSemanticsInterceptor{rule: rule}
}

// Intercept 先 Proceed，2xx 原样放行，非 2xx 套 rule。
// 输入 ch：不改 Request / 响应体。
// 返回：内层错误原样穿透；2xx → resp；rule 返回非 nil → 该 error；rule 返回 nil → 放行 resp。
func (i *statusSemanticsInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	if e := i.rule(resp.StatusCode, resp.Body); e != nil {
		return nil, e
	}
	return resp, nil
}
