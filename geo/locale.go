package geo

import (
	"fmt"
	"strings"
)

// 国家代码 → 主语言 locale 的映射。
//
// 数据源：Meta 官方 77 条 locale 清单 + 各国官方/市场主流第一语言。
// 多语言国家（IN、CH、CA、BE 等）按"单一主语言"原则各选一个；如需精确控制可由调用方
// 显式塞 Config.Locale 覆盖（Locale > CountryCode）。
//
// 收录覆盖：北美、拉美、西欧、北欧、东欧/巴尔干/波罗的海、中东/北非、东亚、东南亚、
// 南亚、撒哈拉以南非洲主要国家、独联体。共 ~110 国，未列入的国家由调用方走兜底
// "en-US,en;q=0.9"。
//
// 关键约束：键必须为 ISO 3166-1 alpha-2 大写形式。本文件运行期不修改 map，
// 不需要锁。

// countryToWebLocale: 国家代码 → BCP 47 web locale（连字符形式，HTTP Accept-Language 用）。
var countryToWebLocale = map[string]string{
	// 英语主导（北美、UK、印太、撒哈拉以南非洲英语主语言国家、加勒比英语小岛、印度洋英语）
	"US": "en-US", "GB": "en-GB", "CA": "en-CA", "AU": "en-AU", "NZ": "en-NZ",
	"IE": "en-IE", "IN": "en-IN", "SG": "en-SG", "PH": "en-PH", "ZA": "en-ZA",
	"NG": "en-NG", "GH": "en-GH", "PK": "en-PK", "JM": "en-JM", "TT": "en-TT",
	"MT": "en-MT",
	"BS": "en-BS", "AI": "en-AI", "BM": "en-BM", "DM": "en-DM", "GY": "en-GY", "VC": "en-VC",
	"MU": "en-MU", "MV": "en-MV", "MW": "en-MW", "NA": "en-NA", "SC": "en-SC", "SL": "en-SL",
	"ZW": "en-ZW",
	// 加勒比英语 / 北美附属
	"AG": "en-AG", "BB": "en-BB", "BZ": "en-BZ", "GD": "en-GD",
	"KN": "en-KN", "KY": "en-KY", "LC": "en-LC", "TC": "en-TC",
	"VG": "en-VG", "VI": "en-VI",
	// 大洋洲 / 太平洋英语
	"FJ": "en-FJ", "GU": "en-GU", "PG": "en-PG", "PW": "en-PW", "WS": "en-WS",
	// 不列颠群岛附属
	"GG": "en-GG", "IM": "en-IM", "JE": "en-JE",
	// 直布罗陀
	"GI": "en-GI",
	// 撒哈拉以南非洲英语扩展
	"BW": "en-BW", "GM": "en-GM", "LR": "en-LR", "LS": "en-LS",
	"SS": "en-SS", "SZ": "en-SZ", "ZM": "en-ZM",

	// 东亚
	"CN": "zh-CN", "TW": "zh-TW", "HK": "zh-HK", "MO": "zh-HK",
	"JP": "ja-JP", "KR": "ko-KR", "MN": "mn-MN",

	// 东南亚
	"ID": "id-ID", "TH": "th-TH", "VN": "vi-VN", "MY": "ms-MY",
	"KH": "km-KH", "LA": "lo-LA", "MM": "my-MM", "BN": "ms-BN",

	// 南亚（IN 已在英语区）
	"BD": "bn-BD", "LK": "si-LK", "NP": "ne-NP", "AF": "ps-AF", "BT": "dz-BT",

	// 西欧
	"FR": "fr-FR", "DE": "de-DE", "IT": "it-IT", "ES": "es-ES", "PT": "pt-PT",
	"NL": "nl-NL", "BE": "nl-BE", "LU": "fr-LU", "CH": "de-CH", "AT": "de-AT",
	"LI": "de-LI", "MC": "fr-MC", "AD": "ca-AD", "SM": "it-SM", "VA": "it-VA",
	"CW": "nl-CW", "AW": "nl-AW", "SX": "nl-SX", "BQ": "nl-BQ",
	// 荷语苏里南
	"SR": "nl-SR",

	// 拉美
	"BR": "pt-BR", "MX": "es-MX", "AR": "es-AR", "CL": "es-CL", "CO": "es-CO",
	"PE": "es-PE", "VE": "es-VE", "EC": "es-EC", "BO": "es-BO", "UY": "es-UY",
	"PY": "es-PY", "CR": "es-CR", "DO": "es-DO", "GT": "es-GT", "HN": "es-HN",
	"NI": "es-NI", "PA": "es-PA", "SV": "es-SV", "CU": "es-CU", "PR": "es-PR",

	// 法属海外（加勒比 / 南美 / 北大西洋 / 太平洋）
	"GF": "fr-GF", "GP": "fr-GP", "HT": "fr-HT", "MF": "fr-MF", "MQ": "fr-MQ",
	"PM": "fr-PM", "NC": "fr-NC", "PF": "fr-PF", "WF": "fr-WF",

	// 北欧
	"DK": "da-DK", "SE": "sv-SE", "NO": "nb-NO", "FI": "fi-FI",
	"IS": "is-IS", "FO": "fo-FO", "GL": "kl-GL",

	// 东欧 / 巴尔干 / 波罗的海
	"RU": "ru-RU", "UA": "uk-UA", "BY": "be-BY", "PL": "pl-PL", "CZ": "cs-CZ",
	"SK": "sk-SK", "HU": "hu-HU", "RO": "ro-RO", "MD": "ro-MD", "BG": "bg-BG",
	"GR": "el-GR", "CY": "el-CY", "HR": "hr-HR", "RS": "sr-RS", "SI": "sl-SI",
	"BA": "bs-BA", "MK": "mk-MK", "AL": "sq-AL", "ME": "sr-ME", "XK": "sq-XK",
	"LT": "lt-LT", "LV": "lv-LV", "EE": "et-EE",

	// 中东 / 高加索
	"TR": "tr-TR", "IL": "he-IL", "IR": "fa-IR",
	"SA": "ar-SA", "AE": "ar-AE", "EG": "ar-EG", "MA": "ar-MA", "DZ": "ar-DZ",
	"TN": "ar-TN", "JO": "ar-JO", "LB": "ar-LB", "KW": "ar-KW", "QA": "ar-QA",
	"BH": "ar-BH", "OM": "ar-OM", "IQ": "ar-IQ", "YE": "ar-YE", "LY": "ar-LY",
	"SD": "ar-SD", "SY": "ar-SY", "PS": "ar-PS",
	"AM": "hy-AM", "AZ": "az-AZ", "GE": "ka-GE",
	// 毛里塔尼亚阿语
	"MR": "ar-MR",

	// 中亚
	"KZ": "kk-KZ", "UZ": "uz-UZ", "KG": "ky-KG", "TJ": "tg-TJ", "TM": "tk-TM",

	// 撒哈拉以南非洲（英语国家已上面列）
	"KE": "sw-KE", "TZ": "sw-TZ", "UG": "sw-UG", "ET": "am-ET", "RW": "rw-RW",
	"SN": "fr-SN", "CI": "fr-CI", "CM": "fr-CM", "MG": "mg-MG",
	"BF": "fr-BF", "BJ": "fr-BJ", "CD": "fr-CD", "CG": "fr-CG", "GN": "fr-GN", "ML": "fr-ML",
	"AO": "pt-AO", "CV": "pt-CV", "MZ": "pt-MZ", "ST": "pt-ST",
	// 法语非洲扩展 / 西葡非洲 / 索马里语 / 印度洋法语
	"BI": "fr-BI", "CF": "fr-CF", "DJ": "fr-DJ", "GA": "fr-GA",
	"NE": "fr-NE", "TD": "fr-TD", "TG": "fr-TG",
	"GQ": "es-GQ", "GW": "pt-GW",
	"SO": "so-SO",
	"KM": "fr-KM", "RE": "fr-RE", "YT": "fr-YT",
}

// countryToMobileLocale: 同上，但用下划线（移动端 app-locale 类 header 的常见格式）。
// 与 countryToWebLocale 一一对应：xx-XX → xx_XX。
var countryToMobileLocale = func() map[string]string {
	m := make(map[string]string, len(countryToWebLocale))
	for cc, web := range countryToWebLocale {
		m[cc] = strings.ReplaceAll(web, "-", "_")
	}
	return m
}()

// WebAcceptLanguageForCountry 根据国家代码返回 Web 端真实格式 Accept-Language 串。
//
// 格式规则（与真实 Chrome 抓包对齐）：
//   - 国家不在表里：返回 ""，由调用方决定兜底
//   - 主语言为 en（en-XX）：输出 "{primary},en;q=0.9"，不叠 en-US 兜底
//     （真实英语用户的 Chrome 默认值即如此，叠 en-US 反而像伪造）
//   - 主语言非 en：输出 "{primary},{lang};q=0.9,en-US;q=0.8,en;q=0.7"
//     （与 scratch_39.txt 真实印尼样本严格一致）
//
// normalizeCountryKey 归一化国家代码为查表 key（trim + 大写）。
// WebAcceptLanguageForCountry / MobileLocaleForCountry / TimezoneOffsetForCountry
// 共用同一归一口径，避免三处各写一遍导致漂移。
func normalizeCountryKey(country string) string {
	return strings.ToUpper(strings.TrimSpace(country))
}

// toBCP47 / toUnderscore 把 locale 归一为「连字符 / 下划线」两种格式（先 trim）。
// 单点化两端 header 派生里反复出现的 trim + 方向替换，避免各处各写一遍导致漂移。
func toBCP47(locale string) string { return strings.ReplaceAll(strings.TrimSpace(locale), "_", "-") }
func toUnderscore(locale string) string {
	return strings.ReplaceAll(strings.TrimSpace(locale), "-", "_")
}

// WebAcceptLanguageForCountry 国家代码大小写都接受（内部 ToUpper）。
// 空串/未命中均返回 ""（normalizeCountryKey + 查表自然处理）。
func WebAcceptLanguageForCountry(country string) string {
	cc := normalizeCountryKey(country)
	web, ok := countryToWebLocale[cc]
	if !ok {
		return ""
	}
	return formatWebAcceptLanguage(web)
}

// MobileLocaleForCountry 根据国家代码返回 Android x-ig-app-locale / x-ig-device-locale
// 等字段用的下划线 locale。国家不在表里（含空串）返回 ""。
func MobileLocaleForCountry(country string) string {
	return countryToMobileLocale[normalizeCountryKey(country)]
}

// localeToWebAcceptLanguage 由显式 locale 派生 Accept-Language。
// 接受 "id_ID" 或 "id-ID"，格式不能识别时返回 ""。
// 用于 BuildWebHeaders 在 Config.Locale 显式覆盖路径下的转换。
func localeToWebAcceptLanguage(locale string) string {
	if locale == "" {
		return ""
	}
	web := toBCP47(locale)
	if !looksLikeBCP47(web) {
		return ""
	}
	return formatWebAcceptLanguage(web)
}

// WebAcceptLanguage 按优先级从地理来源派生 Web 端 Accept-Language。
// 优先级：locale 显式 > countryCode > proxy URL 解析国家。
// 三者都派生不出时返回 ""——不回落 Chrome Profile 锁定的 en-US 或硬编码默认，
// 由调用方（internal.NewClient 构造校验）拦截报错。
func WebAcceptLanguage(locale, countryCode, proxyURL string) string {
	if locale != "" {
		if al := localeToWebAcceptLanguage(locale); al != "" {
			return al
		}
	}
	if countryCode != "" {
		if al := WebAcceptLanguageForCountry(countryCode); al != "" {
			return al
		}
	}
	if cc := ParseCountryFromProxyURL(proxyURL); cc != "" {
		if al := WebAcceptLanguageForCountry(cc); al != "" {
			return al
		}
	}
	return ""
}

// primaryLanguage 取 BCP 47 locale 的主语言段（"id-ID" → "id"；无连字符时原样返回）。
// Web 与 Android 的 accept-language 派生共用本函数做主语言判定，但各自保留输出格式
// （Web 是 ;q= 权重格式、Android 是空格分隔格式，源自两端不同的真实抓包，并非重复实现）。
func primaryLanguage(bcp47 string) string {
	if idx := strings.Index(bcp47, "-"); idx > 0 {
		return bcp47[:idx]
	}
	return bcp47
}

// formatWebAcceptLanguage 拼装 Accept-Language 字符串。
// web 形如 "id-ID" 或 "en-US"。当主语言为 en 时跳过 en-US/en 兜底。
func formatWebAcceptLanguage(web string) string {
	primary := web
	lang := primaryLanguage(web)
	if lang == "en" {
		// en-XX：真实 Chrome 不叠 en-US 兜底
		if primary == "en" {
			return "en;q=0.9"
		}
		return fmt.Sprintf("%s,en;q=0.9", primary)
	}
	return fmt.Sprintf("%s,%s;q=0.9,en-US;q=0.8,en;q=0.7", primary, lang)
}

// looksLikeBCP47 简单校验：非空，仅含字母/数字/连字符，至少 2 字符。
// 不做完整 BCP 47 校验（成本高、收益小），过滤掉明显的脏输入即可。
func looksLikeBCP47(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
