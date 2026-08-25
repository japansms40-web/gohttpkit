package httpx

import (
	"fmt"
	"strings"

	"github.com/japansms40-web/gohttpkit/errors"
)

// interceptors_opt.go —— 可选拦截器：默认链里【没有】它们，需要哪个自己加。
//
// 这些层都带业务判断，而业务判断没有放之四海皆准的版本。把它们做成可选，
// 是为了让「基建替我做了什么」始终是显式的、可读的、可改的。

// ─────────────────────────── 非 2xx 的状态语义 ───────────────────────────

// StatusRule 决定一个非 2xx 响应该被翻译成什么错误。
//
// 返回 nil     → 放行：响应原样继续向外流（调用方自己解析体 + 读状态码）
// 返回非 nil   → 整次调用以该 error 失败
type StatusRule func(status int, body []byte) error

// DefaultStatusRule 默认规则：非 2xx 且响应体为空 → *errors.HTTPStatusError；
// 响应体非空则放行（把体交给调用方，因为大量私有 API 用 4xx 承载有意义的业务响应）。
func DefaultStatusRule(status int, body []byte) error {
	if len(body) > 0 {
		return nil
	}
	return &errors.HTTPStatusError{StatusCode: status, Body: body}
}

// RetryableTextRule 生成一条规则：非 2xx 响应体命中任一关键词，或状态码在 retryStatuses 里，
// 就包成 *errors.RetryableError（调用方 errors.As 后可按「稍后重试」处理，如换代理续跑）；
// 否则回落 DefaultStatusRule。
//
// 用途：服务端限流常常不是 429，而是某个自定义状态码 + 一句人话文案。
func RetryableTextRule(keywords []string, retryStatuses ...int) StatusRule {
	statusSet := make(map[int]struct{}, len(retryStatuses))
	for _, s := range retryStatuses {
		statusSet[s] = struct{}{}
	}
	return func(status int, body []byte) error {
		hit := false
		text := string(body)
		for _, kw := range keywords {
			if kw != "" && strings.Contains(text, kw) {
				hit = true
				break
			}
		}
		if _, ok := statusSet[status]; ok {
			hit = true
		}
		if hit {
			cause := fmt.Errorf("http error: %d, body: %s", status, text)
			return &errors.RetryableError{Err: cause, Attempts: 1, LastError: cause}
		}
		return DefaultStatusRule(status, body)
	}
}

type statusSemanticsInterceptor struct{ rule StatusRule }

// NewStatusSemanticsInterceptor 对非 2xx 响应套用 rule（nil = DefaultStatusRule）。
// 放在链最外层附近：它要看的是经过提纯/改写之后、最终对外的响应体。
func NewStatusSemanticsInterceptor(rule StatusRule) Interceptor {
	if rule == nil {
		rule = DefaultStatusRule
	}
	return &statusSemanticsInterceptor{rule: rule}
}

func (i *statusSemanticsInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	if e := i.rule(resp.StatusCode, resp.Body); e != nil {
		return nil, e
	}
	return resp, nil
}

// ─────────────────────────── 业务错误归类 ───────────────────────────

type classifyInterceptor struct {
	classify func(status int, body []byte) error
}

// NewClassifyInterceptor 把「响应体里的业务错误」统一翻译成 error。
//
// 放在链【最外层】：只有内层判定为成功（返回 resp, nil）时才轮到它，
// 因此它不会覆盖状态语义层已经给出的结论。
//
// 实现建议：2xx 与非 2xx 用不同强度的规则。2xx 响应体常常承载用户生成内容
// （帖子正文、私信、搜索结果），拿自然语言关键词去扫会被正文误伤 —— 那种「偶发地
// 把成功当失败」的 bug 能查一整天。2xx 只匹配结构化字段，非 2xx 才上全量规则。
func NewClassifyInterceptor(classify func(status int, body []byte) error) Interceptor {
	return &classifyInterceptor{classify: classify}
}

func (i *classifyInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if i.classify != nil {
		if e := i.classify(resp.StatusCode, resp.Body); e != nil {
			return nil, e
		}
	}
	return resp, nil
}

// ─────────────────────────── HTML 文本提纯 ───────────────────────────

type htmlTextInterceptor struct{ errorPageMarkers []string }

// NewHTMLTextInterceptor 对 content-type 含 text/html 的响应做纯文本提取
// （见 ExtractHTMLText 的使用警告：它会剥掉 script）。
//
// errorPageMarkers 非空时，响应体命中任一标记即整次调用以错误失败 ——
// 用于识别「HTTP 200 的错误页」这类只能靠页面文案判断的情况。
func NewHTMLTextInterceptor(errorPageMarkers ...string) Interceptor {
	return &htmlTextInterceptor{errorPageMarkers: errorPageMarkers}
}

func (i *htmlTextInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "text/html") {
		return resp, nil
	}
	text := string(resp.Body)
	for _, marker := range i.errorPageMarkers {
		if marker != "" && strings.Contains(text, marker) {
			return nil, fmt.Errorf("http error: %d, body: %s", resp.StatusCode, marker)
		}
	}
	resp.Body = []byte(ExtractHTMLText(text))
	return resp, nil
}

// ─────────────────────────── 请求改写 ───────────────────────────

// NewRequestMutatorInterceptor 在请求发出【之前】就地改写 *http.Request。
// 必须插在 bridge 之后（更靠近网络）才能拿到已构建好的 HTTPReq，
// 用 SpliceBeforeTerminal 插到终端之前即可。
//
// 用途：删掉某个头（ExtraHeaders 的空值是跳过而非删除，删头只能在这里做）、
// 加签名（需要对最终 URL + body 计算）、按 Attempt 换出口。
func NewRequestMutatorInterceptor(mutate func(*Request)) Interceptor {
	return InterceptorFunc(func(ch *Chain) (*Response, error) {
		if mutate != nil {
			mutate(ch.Request())
		}
		return ch.Proceed()
	})
}
