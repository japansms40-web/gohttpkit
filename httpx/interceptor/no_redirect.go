package interceptor

import (
	"net/http"

	"github.com/japansms40-web/gohttpkit/httpx"
)

type noRedirectCallServerInterceptor struct{ httpx.TerminalMarker }

// NewNoRedirectCallServerInterceptor 禁重定向的终端拦截器。
// 给登录/授权链：用浅拷贝 *http.Client（只覆盖 CheckRedirect）发请求，把 302 原样交给调用方。
// 输入：无。返回可放入链的 Interceptor。
func NewNoRedirectCallServerInterceptor() httpx.Interceptor {
	return &noRedirectCallServerInterceptor{}
}

// Intercept 走 doHTTP，使用本次栈上的禁重定向 Client 副本。
// 输入 ch：同 doHTTP；不改共享 HTTPClient，不调用 Proceed。
// 返回：3xx 也是成功 *Response；网络失败 *httpx.TransportError。
func (i *noRedirectCallServerInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	nr := *ch.Client().HTTPClient
	nr.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return doHTTP(ch, &nr)
}
