package interceptor

import (
	"net/http"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// doHTTP 终端共用路径：快照请求头 → Do → 网络错误统一包成 *httpx.TransportError。
// 给两种终端：必须走这里，否则一边忘了包装，那条链上的重试会静默失效。
func doHTTP(ch *httpx.Chain, client *http.Client) (*httpx.Response, error) {
	httpx.SnapshotRequestHeaders(ch.Request())
	//nolint:bodyclose // 所有权移交 bodyDecodeInterceptor，由它读完体后 Close
	resp, err := client.Do(ch.Request().HTTPReq)
	if err != nil {
		return nil, &httpx.TransportError{Err: err}
	}
	return newResponseFrom(resp), nil
}

func newResponseFrom(resp *http.Response) *httpx.Response {
	return &httpx.Response{
		StatusCode: resp.StatusCode,
		Proto:      resp.Proto,
		ProtoMajor: resp.ProtoMajor,
		Header:     resp.Header,
		Raw:        resp,
	}
}
