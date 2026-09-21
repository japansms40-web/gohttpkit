package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

type classifyInterceptor struct {
	classify func(status int, body []byte) error
}

// NewClassifyInterceptor 把「响应体里的业务错误」统一翻译成 error。
// 给 JSON API：放链最外层，只有内层成功时才轮到它，不会覆盖状态语义层的结论。
// 输入 classify：可为 nil（Intercept 直接放行）。返回可放入链的 Interceptor。
func NewClassifyInterceptor(classify func(status int, body []byte) error) httpx.Interceptor {
	return &classifyInterceptor{classify: classify}
}

// Intercept 先 Proceed，成功后再跑 classify。
// 输入 ch：不改 Request / 响应体。
// 返回：内层错误原样穿透；classify==nil 或返回 nil → resp；classify 非 nil → 该 error。
func (i *classifyInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if i.classify != nil {
		if e := i.classify(resp.StatusCode, resp.Body); e != nil {
			return nil, e
		}
	}
	return resp, nil
}
