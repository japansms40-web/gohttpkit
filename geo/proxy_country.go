package geo

import (
	"net/url"
	"regexp"
	"strings"
)

// proxy_country.go —— 从代理 URL 的 userinfo 解析出口国家。
// 独立成文件：解析规则跟 locale/时区大表无关，混在一起会让「改正则」和「改国家表」搅在同一 diff。

// rolaCountryRegex 匹配 rola.vip 风格代理用户名里的 country-xx 段。
//
// 真实样本：socks5://new001kOtD8x_559143-sesstime-20-country-us:pwd@gate5.rola.vip:2031
// 用户名格式：account_session-sesstime-NN-country-xx
//
// 后界 [-_]|$ 防止 country-united-states 这种长串误命中前两位 "un"。
var rolaCountryRegex = regexp.MustCompile(`country-([a-zA-Z]{2})(?:[-_]|$)`)

// ParseCountryFromProxyURL 从代理 URL 的 userinfo 解析 country-xx。
// 输入 rawURL：SOCKS5/HTTP 代理 URL 原文。
// 返回：命中是大写两位（"US"）；空串、畸形 URL、无 userinfo、无 country-xx 都返回 ""。
// 例：`socks5://acc-country-us:pwd@h:1` → "US"；`http://h:1` → ""。
// 给 CountryCode 为空时做出口国 fallback。用户名优先、密码兜底。
// 不保证两位字母是合法 ISO 国家，调用方再喂给查表函数校验。
func ParseCountryFromProxyURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return ""
	}
	// 用户名优先，密码段兜底：两段都可能编 country-xx，取决于代理商。
	// rola 编在用户名（account-sesstime-NN-country-us），evomi 要求编在密码
	// （base_country-BR_session-NN，见 InsExecutor pkg/proxy/url_builder.go
	// 的 Password 模板）。只扫用户名会让 evomi 这类静默落空，整条画像派生链
	// 跟着废掉——2026-08-17 生产事故就是这么来的：出口 us、Country 解析成空、
	// AcceptLanguage 兜底 zh-CN，服务端按中文 locale 判定受限地区，直接拒绝请求。
	if cc := matchCountryTag(u.User.Username()); cc != "" {
		return cc
	}
	password, _ := u.User.Password()
	return matchCountryTag(password)
}

// matchCountryTag 从一段凭据文本里抓 country-xx。
// 输入 s：用户名或密码原文。
// 返回：命中是大写两位；空串或不匹配返回 ""。
func matchCountryTag(s string) string {
	if s == "" {
		return ""
	}
	m := rolaCountryRegex.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.ToUpper(m[1])
}
