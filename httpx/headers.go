package httpx

import (
	"context"
	"strings"
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

// BuildHeaders 实现 HeaderProvider（返回副本，避免链上改写污染原表）。
func (s StaticHeaders) BuildHeaders(context.Context) map[string]string {
	out := make(map[string]string, len(s.Headers))
	for k, v := range s.Headers {
		out[strings.ToLower(k)] = v
	}
	return out
}

// BaseURL 实现 HeaderProvider。
func (s StaticHeaders) BaseURL() string { return s.Base }

// HeaderProviderFunc 把「函数 + 域名」适配成 HeaderProvider。
type HeaderProviderFunc struct {
	Base  string
	Build func(ctx context.Context) map[string]string
}

// BuildHeaders 实现 HeaderProvider。
func (f HeaderProviderFunc) BuildHeaders(ctx context.Context) map[string]string {
	if f.Build == nil {
		return map[string]string{}
	}
	return f.Build(ctx)
}

// BaseURL 实现 HeaderProvider。
func (f HeaderProviderFunc) BaseURL() string { return f.Base }

// FilterHeadersByWhitelist 按白名单过滤 headers（大小写不敏感）。
//
// whitelist 的 key 是要发送的 header 名，value 是取值策略：
//   - value 为空串 ""：用 allHeaders 里构建出的值（找不到就整条跳过，不发空头）
//   - value 非空：直接用白名单里配置的固定值，忽略构建值
//
// 注意 nil 与空 map 的区别由调用方（bridge）区分：本函数收到空 whitelist 一律返回空 map。
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
// origin 取 baseURL（协议 + 域名），referer 取 baseURL + path（不含查询参数）。
func BuildOriginAndReferer(baseURL, path string) (origin, referer string) {
	return baseURL, baseURL + path
}
