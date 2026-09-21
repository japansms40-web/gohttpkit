package httpx

import (
	"context"
	"net/http"
	"strings"
)

// headers.go —— HeaderProvider 接缝、白名单过滤、origin/referer 派生。
// 独立成文件：发哪些头是本库最容易被误改的契约，和链框架、拦截器实现分开便于评审。

const (
	headerContentType = "content-type"
	mimeJSON          = "application/json"
	mimeHTML          = "text/html"
)

// HeaderProvider 是本库最重要的接缝：请求头怎么构建完全交给接入方。
//
// 库自己不知道任何业务字段（cookie 怎么拼、UA 怎么选、csrf 从哪来、会话状态怎么轮转），
// 只负责「把 BuildHeaders 给的候选头按白名单过滤后发出去」。想复刻某个真实客户端的
// 请求指纹，就在这个接口的实现里做；库不会替你猜。
//
// 并发要求：同一个 *Client 可能被多 goroutine 并发使用，BuildHeaders 必须并发安全。
// 带会话状态（csrftoken 回写、UA 缓存）的实现请自带锁，见 examples/fidelity。
type HeaderProvider interface {
	// BuildHeaders 返回本次请求的【全量候选头】。
	//
	// 约定 key 全小写：本库绕过 http.Header.Set 的 CanonicalMIMEHeaderKey 直写底层 map，
	// 真实浏览器与移动端在 HTTP/2 上发的就是小写头，大写会成为指纹差异。
	// 返回 nil 表示构头失败，请求直接以错误返回、不发出网络请求。
	BuildHeaders(ctx context.Context) map[string]string

	// BaseURL 返回默认域名（含 scheme，不带尾斜杠），如 "https://api.example.com"。
	// Request.Path 传绝对 URL 时 FullURL 用 Path，但 origin / referer 仍按 BaseURL 算
	// —— 跨站请求的 origin 本就该指向发起站点，与真实浏览器行为一致。
	BaseURL() string
}

// StaticHeaders 是最简 HeaderProvider：一组固定头 + 固定域名。
// 适合内部服务调用、快速起步；需要会话状态请自己实现 HeaderProvider。
type StaticHeaders struct {
	// Base 基础域名，如 "https://api.example.com"。
	Base string
	// Headers 候选头（key 请用小写）。返回时会拷贝一份，调用方后续修改不影响已发请求。
	Headers map[string]string
}

// BuildHeaders 实现 HeaderProvider：返回候选头副本。
// 给固定头场景：内部服务、快速起步。
// 输入 ctx：忽略；s.Headers 为 nil 时返回空 map（非 nil）。
// 返回：新 map，key 一律小写；调用方后续改副本不影响 StaticHeaders.Headers。
// 例：StaticHeaders{Headers: map[string]string{"Accept":"*"}}.BuildHeaders(ctx)["accept"] == "*"。
func (s StaticHeaders) BuildHeaders(context.Context) map[string]string {
	out := make(map[string]string, len(s.Headers))
	for k, v := range s.Headers {
		out[strings.ToLower(k)] = v
	}
	return out
}

// BaseURL 实现 HeaderProvider。
// 输入：无。返回 s.Base 原串，空串表示未配域名。
// 例：StaticHeaders{Base: "https://api.example.com"}.BaseURL() → 该串。
func (s StaticHeaders) BaseURL() string { return s.Base }

// HeaderProviderFunc 把「函数 + 域名」适配成 HeaderProvider。
type HeaderProviderFunc struct {
	// Base 基础域名，语义同 HeaderProvider.BaseURL。
	Base string
	// Build 构头函数。nil 时 BuildHeaders 返回空 map（不是 nil），避免被 bridge
	// 判成「构头失败」——没配函数应表示「没有任何头」，而不是整次调用报错。
	Build func(ctx context.Context) map[string]string
}

// BuildHeaders 实现 HeaderProvider。
// 给「函数 + 域名」适配：闭包里带会话状态即可，不必再定义类型。
// 输入 ctx：原样交给 Build；Build == nil 返回非 nil 空 map（不是构头失败）。
// 返回：非 nil Build 的返回值原样透传，包括 nil —— bridge 会把 nil 判成 *NilBuildHeadersError。
// 例：HeaderProviderFunc{}.BuildHeaders(ctx) → map[string]string{}；
// Build 自己返回 nil → 原样 nil。
func (f HeaderProviderFunc) BuildHeaders(ctx context.Context) map[string]string {
	if f.Build == nil {
		return map[string]string{}
	}
	return f.Build(ctx)
}

// BaseURL 实现 HeaderProvider。
// 输入：无。返回 f.Base 原串，空串表示未配域名。
// 例：HeaderProviderFunc{Base: "https://api.example.com"}.BaseURL() → 该串。
func (f HeaderProviderFunc) BaseURL() string { return f.Base }

// FilterHeadersByWhitelist 按白名单过滤 headers（大小写不敏感）。
// 给 bridge 与自组发头逻辑：从全量候选头里挑出真正要发的那一组。
// 输入 allHeaders：构头结果，可为 nil（视为空表）；本函数不改它。
// 输入 whitelist：key 是要发送的头名；value 空串取构建值，非空用固定值覆盖。
// 返回：新 map；whitelist 为 nil 或空 map 一律返回空 map（不返回 nil）。
// 例：whitelist{"accept":""} + allHeaders{"accept":"*/*"} → {"accept":"*/*"}；
// whitelist 为空 → map[string]string{}。
// nil 与空 map 的「全量 vs 严格」区别由调用方（bridge）区分，本函数不负责。
func FilterHeadersByWhitelist(allHeaders, whitelist map[string]string) map[string]string {
	if len(whitelist) == 0 {
		return make(map[string]string)
	}

	// 快路径：构头与白名单的 key 约定都是全小写，直接命中 allHeaders 即可；
	// 仅 miss 时才回退大小写不敏感慢查找（惰性构建一次小写索引）。
	// 每请求（含每次重试）都会走这里，省掉几十条 entry 的索引 map 常态分配。
	var lowerIndex map[string]string
	lookupInsensitive := func(lowerKey string) (string, bool) {
		if lowerIndex == nil {
			lowerIndex = make(map[string]string, len(allHeaders))
			for key, value := range allHeaders {
				lowerIndex[strings.ToLower(key)] = value
			}
		}
		v, ok := lowerIndex[lowerKey]
		return v, ok
	}

	filtered := make(map[string]string, len(whitelist))
	for whitelistKey, defaultValue := range whitelist {
		if defaultValue != "" {
			filtered[whitelistKey] = defaultValue
			continue
		}
		value, exists := allHeaders[whitelistKey]
		if !exists {
			value, exists = lookupInsensitive(strings.ToLower(whitelistKey))
		}
		if exists {
			filtered[whitelistKey] = value
		}
		// allHeaders 里也没有 → 跳过，不发送该头
	}
	return filtered
}

// BuildOriginAndReferer 按 baseURL 与 path 构建 origin / referer。
// 给 bridge：默认注入候选头；DisableOriginReferer 时不会走到这里。
// 输入 baseURL：协议 + 域名，不带尾斜杠；空串时 origin/referer 都从空串拼。
// 输入 path：请求路径或绝对 URL；不含查询参数（查询在 FullURL 上，不进 referer）。
// 返回：origin=baseURL，referer=baseURL+path；不做合法性校验。
// 例：("https://api.example.com", "/v1") → ("https://api.example.com", "https://api.example.com/v1")。
func BuildOriginAndReferer(baseURL, path string) (origin, referer string) {
	return baseURL, baseURL + path
}

// HeaderIsHTML 判定响应是否按 HTML 处理。
// 给 htmlSave / htmlText 与自定义观察层：只认 content-type 子串，不解析参数表。
// 输入 h：响应头，nil 返回 false。
// 返回：小写 content-type 含 "text/html" 为 true；缺头或其它类型为 false。
// 线上常见 "text/html; charset=utf-8"，完整解析对这一个判断没有收益。
func HeaderIsHTML(h http.Header) bool {
	if h == nil {
		return false
	}
	return strings.Contains(strings.ToLower(h.Get(headerContentType)), mimeHTML)
}

// SnapshotRequestHeaders 在第一次尝试时留一份「最终发出的请求头」快照到 Request.ReqHeaders。
// 给终端拦截器与自定义终端：发出前调用，否则日志里的 req_headers 会是空的。
// 输入 req：可为 nil（直接返回）；Attempt!=0 或 HTTPReq==nil 也不写。
// 返回：无；写入 req.ReqHeaders（新 Header）。不改 HTTPReq.Header 内容。
func SnapshotRequestHeaders(req *Request) {
	if req == nil || req.Attempt != 0 || req.HTTPReq == nil {
		return
	}
	snapshot := make(http.Header, len(req.HTTPReq.Header)+1)
	for k, v := range req.HTTPReq.Header {
		if k == "User-Agent" && len(v) == 1 && v[0] == "" {
			continue
		}
		snapshot[k] = v
	}
	if req.HTTPReq.Host != "" {
		snapshot["host"] = []string{req.HTTPReq.Host}
	} else if req.HTTPReq.URL != nil {
		snapshot["host"] = []string{req.HTTPReq.URL.Host}
	}
	req.ReqHeaders = snapshot
}
