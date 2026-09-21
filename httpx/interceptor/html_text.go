package interceptor

import (
	"strings"

	"github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
)

type htmlTextInterceptor struct{ errorPageMarkers []string }

// NewHTMLTextInterceptor 对 HTML 响应做纯文本提取（会剥掉 script，见 ExtractHTMLText）。
// 给只要可见文案的页面流；需要抠 token 的场景不要加。
// 输入 errorPageMarkers：非空时命中任一标记即失败；空串条目忽略。
// 返回：可放入链的 Interceptor。
func NewHTMLTextInterceptor(errorPageMarkers ...string) httpx.Interceptor {
	return &htmlTextInterceptor{errorPageMarkers: errorPageMarkers}
}

// Intercept 先 Proceed，非 HTML 原样放行，HTML 则提纯或按标记失败。
// 输入 ch：命中提纯时原地替换 resp.Body。
// 返回：内层错误原样穿透；非 HTML → 原 resp；命中标记 → *errors.HTTPStatusError；否则提纯后的 resp。
func (i *htmlTextInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if !httpx.HeaderIsHTML(resp.Header) {
		return resp, nil
	}
	text := string(resp.Body)
	for _, marker := range i.errorPageMarkers {
		if marker != "" && strings.Contains(text, marker) {
			return nil, &errors.HTTPStatusError{StatusCode: resp.StatusCode, Body: []byte(marker)}
		}
	}
	resp.Body = []byte(httpx.ExtractHTMLText(text))
	return resp, nil
}
