package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

type statusCodeCacheInterceptor struct{}

// NewStatusCodeCacheInterceptor 记录最近一次响应的状态码，供 SnapshotResponseStatusCode 读。
// 给默认链：位于 retry 之外，重试跑完才记。
// 输入：无。返回可放入链的 Interceptor。
func NewStatusCodeCacheInterceptor() httpx.Interceptor { return &statusCodeCacheInterceptor{} }

// Intercept 先 Proceed，成功后写入状态码缓存。
// 输入 ch：经 CacheStatusCode 改 Client 缓存；不改 Request。
// 返回：内层错误原样穿透（不写缓存）；成功返回同一份 resp。
func (i *statusCodeCacheInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	ch.Client().CacheStatusCode(resp.StatusCode)
	return resp, nil
}
