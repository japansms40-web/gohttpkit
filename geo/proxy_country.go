package geo

import (
	"net/url"
	"regexp"
	"strings"
)

// rolaCountryRegex 匹配 rola.vip 风格代理用户名里的 country-xx 段。
//
// 真实样本：socks5://new001kOtD8x_559143-sesstime-20-country-us:pwd@gate5.rola.vip:2031
// 用户名格式：account_session-sesstime-NN-country-xx
//
// 后界 [-_]|$ 防止 country-united-states 这种长串误命中前两位 "un"。
var rolaCountryRegex = regexp.MustCompile(`country-([a-zA-Z]{2})(?:[-_]|$)`)

// ParseCountryFromProxyURL 从 SOCKS5/HTTP proxy URL 的 userinfo 里解析 country-xx
// 标签——用户名段优先，密码段兜底（不同代理商编码位置不同，见下方实现注释）。
// 命中返回大写 ISO 3166-1 alpha-2（"US"/"KR"/"PR" 等），未命中或畸形 URL 返回 ""。
//
// 用途：当 Config.CountryCode 为空时，SDK 派生层用本函数从 proxy URL 提取出口国家
// 作为 fallback，避免出现"代理 country-us 但 header 全是 zh_CN"的矛盾信号面。
// 调用方（resolveAndroidLocale / resolveAndroidTimezoneOffset / resolveWebAcceptLanguage）
// 把本函数结果再喂给 MobileLocaleForCountry / TimezoneOffsetForCountry / WebAcceptLanguageForCountry
// 二次校验——本函数不保证返回的两位字母是合法 ISO 国家。
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

// matchCountryTag 从一段凭据文本里抓 country-xx，命中返回大写两位，否则空串。
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
