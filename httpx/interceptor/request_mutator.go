package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

// NewRequestMutatorInterceptor 在请求发出之前就地改写 *Request。
// 给删头、加签名、按 Attempt 换出口：必须插在 bridge 之后（SpliceBeforeTerminal）。
// 输入 mutate：可为 nil（Intercept 直接 Proceed）。返回 InterceptorFunc。
func NewRequestMutatorInterceptor(mutate func(*httpx.Request)) httpx.Interceptor {
	return httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		if mutate != nil {
			mutate(ch.Request())
		}
		return ch.Proceed()
	})
}
