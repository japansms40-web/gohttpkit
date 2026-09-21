package interceptor

import (
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
)

type transactionInterceptor struct {
	httpx.SideChannelMarker
	sink func(*httpx.Transaction)
}

// NewTransactionInterceptor 把整链处理后的完整请求/响应快照交给 sink。
// 给调试落盘：放链最外层，标记为 SideChannel，WithChain 会自动带过去。
// 输入 sink：可为 nil（Intercept 不回调）；快照是原文，不要写进共享日志。
// 返回：旁路观察 Interceptor。出错或 resp 为 nil 不回调。
func NewTransactionInterceptor(sink func(*httpx.Transaction)) httpx.Interceptor {
	return &transactionInterceptor{sink: sink}
}

// Intercept 先 Proceed，再按需回调 sink。
// 输入 ch：不改 Request / Client。
// 返回：内层 resp/err 原样返回；sink==nil 或 resp==nil 不回调。
func (i *transactionInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	start := time.Now()
	resp, err := ch.Proceed()
	if i.sink != nil && resp != nil {
		req := ch.Request()
		i.sink(&httpx.Transaction{
			Method:      req.Method,
			URL:         req.FullURL,
			Proxy:       ch.Client().Options().ProxyURL,
			ExitIP:      ch.Client().Options().ExitIP,
			ASN:         ch.Client().Options().ASN,
			ReqHeaders:  map[string][]string(req.ReqHeaders),
			ReqBody:     string(req.Body),
			ReqBodyLen:  len(req.Body),
			Status:      resp.StatusCode,
			RespHeaders: map[string][]string(resp.Header),
			RespBody:    string(resp.Body),
			RespBodyLen: len(resp.Body),
			DurationMS:  time.Since(start).Milliseconds(),
		})
	}
	return resp, err
}
