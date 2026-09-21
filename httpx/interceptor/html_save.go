package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

type htmlSaveInterceptor struct {
	httpx.SideChannelMarker
	sink func([]byte)
}

// NewHTMLSaveInterceptor 只保存 HTML 响应。
// 给需要落盘 HTML 的调用方：content-type 含 text/html 时把响应体交给 sink。
// 输入 sink：可为 nil（不保存）。返回 SideChannel 拦截器；不修改响应。
func NewHTMLSaveInterceptor(sink func(html []byte)) httpx.Interceptor {
	return &htmlSaveInterceptor{sink: sink}
}

// Intercept 先 Proceed，仅在成功且是 HTML 时回调 sink。
// 输入 ch：不改 Request / 响应体。
// 返回：内层错误原样返回（出错不保存）；sink==nil、resp==nil 或非 HTML 不回调。
func (i *htmlSaveInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return resp, err
	}
	if i.sink != nil && resp != nil && httpx.HeaderIsHTML(resp.Header) {
		i.sink(resp.Body)
	}
	return resp, nil
}
