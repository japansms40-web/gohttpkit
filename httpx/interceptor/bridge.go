package interceptor

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/japansms40-web/gohttpkit/httpx"
)

type bridgeInterceptor struct{}

// NewBridgeInterceptor 每次尝试重建 *http.Request 并构建/过滤请求头（OkHttp BridgeInterceptor 对位）。
// 给默认链：必须位于 retry 之内，否则重试时用的是过期的头（比如已经轮转过的 token）。
// 输入：无。返回可放入链的 Interceptor。
func NewBridgeInterceptor() httpx.Interceptor { return &bridgeInterceptor{} }

// Intercept 重建 HTTPReq、构头、过滤后再 Proceed。
// 输入 ch：写入 Request.HTTPReq；不改 Client。
// 返回：建请求失败 → *httpx.CreateHTTPRequestError（服务器计数为 0）；
// BuildHeaders 返回 nil → *httpx.NilBuildHeadersError；否则是内层 Proceed 的结果。
func (i *bridgeInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	req := ch.Request()
	c := ch.Client()

	var reqBody io.Reader
	if len(req.Body) > 0 {
		reqBody = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(req.Ctx, req.Method, req.FullURL, reqBody)
	if err != nil {
		return nil, &httpx.CreateHTTPRequestError{Method: req.Method, Err: err}
	}

	built := c.Headers().BuildHeaders(req.Ctx)
	if built == nil {
		return nil, &httpx.NilBuildHeadersError{ProviderType: fmt.Sprintf("%T", c.Headers())}
	}

	allHeaders := make(map[string]string, len(built)+2)
	for k, v := range built {
		allHeaders[strings.ToLower(k)] = v
	}

	if !c.Options().DisableOriginReferer {
		origin, referer := httpx.BuildOriginAndReferer(req.BaseURL, req.Path)
		allHeaders["origin"] = origin
		allHeaders["referer"] = referer
	}

	strict := req.HeaderWhitelist != nil
	final := allHeaders
	if strict {
		final = httpx.FilterHeadersByWhitelist(allHeaders, req.HeaderWhitelist)
	}

	for key, value := range req.ExtraHeaders {
		if value == "" {
			continue
		}
		final[strings.ToLower(key)] = value
	}

	applySpecialHeaders(httpReq, final, strict)

	for key, value := range final {
		httpReq.Header[key] = []string{value}
	}

	req.HTTPReq = httpReq
	return ch.Proceed()
}

// applySpecialHeaders 处理标准库特殊对待的头，使「全小写写头」在 HTTP/1.1 与 HTTP/2 下
// 都产生同一份、且不重复的线上字节。
// 给 bridge：final 里被本函数消化掉的条目会被删除。
func applySpecialHeaders(httpReq *http.Request, final map[string]string, strict bool) {
	if v, ok := final["host"]; ok {
		httpReq.Host = v
		delete(final, "host")
	}
	if v, ok := final["content-length"]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			httpReq.ContentLength = n
		}
		delete(final, "content-length")
	}
	_, hasUA := final["user-agent"]
	if hasUA || strict {
		httpReq.Header["User-Agent"] = []string{""}
	}
}
