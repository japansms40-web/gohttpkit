package interceptor

import (
	"net/http"

	"github.com/japansms40-web/gohttpkit/httpx"
)

type responseHeaderCacheInterceptor struct{}

// NewResponseHeaderCacheInterceptor 缓存最近一次响应头，并调用 OnResponseHeaders 回写会话。
// 给默认链：无论状态码如何都会缓存，400/403 上的新凭证不能丢。
// 输入：无。返回可放入链的 Interceptor。
func NewResponseHeaderCacheInterceptor() httpx.Interceptor {
	return &responseHeaderCacheInterceptor{}
}

// Intercept 先 Proceed，成功后复制响应头、写缓存、调 hook。
// 输入 ch：写 Client 头缓存；hook 为 nil 则只缓存不回调。
// 返回：内层错误原样穿透（不缓存、不回调）；成功返回同一份 resp。
func (i *responseHeaderCacheInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}

	newHeaders := make(http.Header, len(resp.Header))
	for key, values := range resp.Header {
		for _, v := range values {
			newHeaders.Add(key, v)
		}
	}
	ch.Client().CacheResponseHeaders(newHeaders)

	if hook := ch.Client().Options().OnResponseHeaders; hook != nil {
		hook(ch.Request().Ctx, newHeaders)
	}
	return resp, nil
}
