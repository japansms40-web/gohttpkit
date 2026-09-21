package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

type callServerInterceptor struct{ httpx.TerminalMarker }

// NewCallServerInterceptor 终端拦截器：发出请求，不调用 Proceed。
// 给默认链最内层。Do 的错误包成 *httpx.TransportError 供 retry 识别。
// 输入：无。返回可放入链的 Interceptor。
// 响应体的所有权在这里【移交】给 bodyDecode（它读完体后负责 Close）。
// 自组链若不含 NewBodyDecodeInterceptor，就必须自己关闭 Response.Raw.Body。
func NewCallServerInterceptor() httpx.Interceptor { return &callServerInterceptor{} }

// Intercept 走 doHTTP，使用共享 HTTPClient。
// 输入 ch：会经 SnapshotRequestHeaders 写 ReqHeaders；不调用 Proceed。
// 返回：成功 *Response（Raw.Body 未读）；网络失败 *httpx.TransportError。
func (i *callServerInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	return doHTTP(ch, ch.Client().HTTPClient)
}
