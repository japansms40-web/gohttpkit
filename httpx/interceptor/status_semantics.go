package interceptor

import (
	"context"

	"github.com/japansms40-web/gohttpkit/httpx"
)

type statusSemanticsInterceptor struct{ rule httpx.StatusRuleContext }

// NewStatusSemanticsInterceptor 对非 2xx 响应套用 rule。
// 给想把空体 4xx/5xx 变成 error 的调用方；放链最外层附近，看最终对外的体。
// 输入 rule：nil 回落 DefaultStatusRule。返回可放入链的 Interceptor。
func NewStatusSemanticsInterceptor(rule httpx.StatusRule) httpx.Interceptor {
	if rule == nil {
		rule = httpx.DefaultStatusRule
	}
	return &statusSemanticsInterceptor{rule: func(_ context.Context, status int, body []byte) error {
		return rule(status, body)
	}}
}

// NewStatusSemanticsInterceptorContext 同 NewStatusSemanticsInterceptor，但规则额外拿到本次请求的 ctx。
// 给命中时要打带 trace_id 的事件日志等需要 ctx 的规则；链上位置与放行语义同 NewStatusSemanticsInterceptor。
// 输入 rule：nil 回落 DefaultStatusRule。返回可放入链的 Interceptor。
func NewStatusSemanticsInterceptorContext(rule httpx.StatusRuleContext) httpx.Interceptor {
	if rule == nil {
		return NewStatusSemanticsInterceptor(nil)
	}
	return &statusSemanticsInterceptor{rule: rule}
}

// Intercept 先 Proceed，2xx 原样放行，非 2xx 套 rule。
// 输入 ch：不改 Request / 响应体；rule 拿到的 ctx 是 ch.Request().Ctx。
// 返回：内层错误原样穿透；2xx → resp；rule 返回非 nil → 该 error；rule 返回 nil → 放行 resp。
func (i *statusSemanticsInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if httpx.IsSuccessStatus(resp.StatusCode) {
		return resp, nil
	}
	if e := i.rule(ch.Request().Ctx, resp.StatusCode, resp.Body); e != nil {
		return nil, e
	}
	return resp, nil
}
