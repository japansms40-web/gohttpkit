package geo

import (
	"sort"
	"strings"

	"math/rand/v2"
)

// locale_web.go —— 国家码 → Chrome data-code，以及按 Chromium 规则拼 Accept-Language。
// 与 Android 表分开维护；keyset 由 TestTimezoneTableKeysetMatchesLocaleTable 对齐。

// countryToWebAcceptTag 国家码 → Chrome「添加语言」data-code（Accept-Language 第一项）。
// 清单有精确项则保留（zh-CN / en-US / de-CH）；只有语种则收成语种（ja-JP → ja，id-ID → id）。
// 值按 Chrome 清单收过一次，写进字面量；运行期不再判断。
var countryToWebAcceptTag = map[string]string{

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
	"JP": "ja", "KR": "ko", "MN": "mn",

	// 东南亚
	"ID": "id", "TH": "th", "VN": "vi", "MY": "ms",
	"KH": "km", "LA": "lo", "MM": "my", "BN": "ms",

	// 南亚（IN 已在英语区）
	"BD": "bn", "LK": "si", "NP": "ne", "AF": "ps", "BT": "dz-BT",

	// 西欧
	"FR": "fr-FR", "DE": "de-DE", "IT": "it-IT", "ES": "es-ES", "PT": "pt-PT",
	"NL": "nl", "BE": "nl", "LU": "fr", "CH": "de-CH", "AT": "de-AT",
	"LI": "de-LI", "MC": "fr", "AD": "ca", "SM": "it", "VA": "it",
	"CW": "nl", "AW": "nl", "SX": "nl", "BQ": "nl",
	// 荷语苏里南
	"SR": "nl",

	// 拉美
	"BR": "pt-BR", "MX": "es-MX", "AR": "es-AR", "CL": "es-CL", "CO": "es-CO",
	"PE": "es-PE", "VE": "es-VE", "EC": "es", "BO": "es", "UY": "es-UY",
	"PY": "es", "CR": "es-CR", "DO": "es", "GT": "es", "HN": "es-HN",
	"NI": "es", "PA": "es", "SV": "es", "CU": "es", "PR": "es",

	// 法属海外（加勒比 / 南美 / 北大西洋 / 太平洋）
	"GF": "fr", "GP": "fr", "HT": "fr", "MF": "fr", "MQ": "fr",
	"PM": "fr", "NC": "fr", "PF": "fr", "WF": "fr",

	// 北欧
	"DK": "da", "SE": "sv", "NO": "nb", "FI": "fi",
	"IS": "is", "FO": "fo", "GL": "kl-GL",

	// 东欧 / 巴尔干 / 波罗的海
	"RU": "ru", "UA": "uk", "BY": "be", "PL": "pl", "CZ": "cs",
	"SK": "sk", "HU": "hu", "RO": "ro", "MD": "ro-MD", "BG": "bg",
	"GR": "el", "CY": "el", "HR": "hr", "RS": "sr", "SI": "sl",
	"BA": "bs", "MK": "mk", "AL": "sq", "ME": "sr", "XK": "sq",
	"LT": "lt", "LV": "lv", "EE": "et",

	// 中东 / 高加索
	"TR": "tr-TR", "IL": "he", "IR": "fa",
	"SA": "ar", "AE": "ar", "EG": "ar", "MA": "ar", "DZ": "ar",
	"TN": "ar", "JO": "ar", "LB": "ar", "KW": "ar", "QA": "ar",
	"BH": "ar", "OM": "ar", "IQ": "ar", "YE": "ar", "LY": "ar",
	"SD": "ar", "SY": "ar", "PS": "ar",
	"AM": "hy", "AZ": "az", "GE": "ka",
	// 毛里塔尼亚阿语
	"MR": "ar",

	// 中亚
	"KZ": "kk", "UZ": "uz", "KG": "ky", "TJ": "tg", "TM": "tk",

	// 撒哈拉以南非洲（英语国家已上面列）
	"KE": "sw", "TZ": "sw", "UG": "sw", "ET": "am", "RW": "rw",
	"SN": "fr", "CI": "fr", "CM": "fr", "MG": "mg",
	"BF": "fr", "BJ": "fr", "CD": "fr", "CG": "fr", "GN": "fr", "ML": "fr",
	"AO": "pt", "CV": "pt", "MZ": "pt", "ST": "pt",
	// 法语非洲扩展 / 西葡非洲 / 索马里语 / 印度洋法语
	"BI": "fr", "CF": "fr", "DJ": "fr", "GA": "fr",
	"NE": "fr", "TD": "fr", "TG": "fr",
	"GQ": "es", "GW": "pt",
	"SO": "so",
	"KM": "fr", "RE": "fr", "YT": "fr",
}

// WebAcceptLanguageForCountry 按国家代码查 Chrome「添加语言」data-code。
// 输入 country：ISO 3166-1 alpha-2，大小写不敏感，自动 trim。
// 返回：表值（ja / zh-CN / en-US）；空输入或未命中返回 ("", *UnknownCountryError)。
// 例："jp" → ("ja", nil)；"cn" → ("zh-CN", nil)；"" → ("", *UnknownCountryError{Country:""})。
func WebAcceptLanguageForCountry(country string) (string, error) {
	return lookupCountry(countryToWebAcceptTag, country)
}

// BuildChromeAcceptLanguage 把按偏好排序、不带 q 值的 Chrome data-code 编成 Accept-Language。
// 输入 tags：调用方排好序的 data-code（如 []string{"zh-CN","en","ja"}），允许首尾空白，不改原切片。
// 返回：Chromium 风格 header；nil、空列表或全空元素返回 ""。去重区分大小写。
// 例：["zh-CN","en"] → "zh-CN,zh;q=0.9,en;q=0.8"；[] → ""。
// 规则对齐 Chromium ExpandLanguageList + GenerateAcceptLanguageHeader：
// 只看紧邻下一项是否同族再决定是否补基础语言；第一项不写 q，其后每步 0.1，下限 0.1。
func BuildChromeAcceptLanguage(tags []string) string {
	return formatChromeAcceptLanguageQ(expandChromeLanguageList(tags))
}

// BuildChromeAcceptLanguageForCountry 以国家对应 data-code 为第一偏好，再随机追加 extra 项后拼 header。
// 输入 country：ISO 3166-1 alpha-2；extra：额外原始 Chrome 项数（不是展开后的 header 项数，抽样不放回）；
// rng：抽样源。extra==0 时不读 rng；nil 时用 math/rand/v2 顶层 Perm（可并发）。
// 返回：成功是完整 Accept-Language；出错是 ("", err)。
// extra<0 → *InvalidExtraLanguageCountError；未知国家 → *UnknownCountryError；
// extra 大于剩余候选 → *ExtraLanguageCountExceedsPoolError。
// 例：("CN", 0, nil) → ("zh-CN,zh;q=0.9", nil)。
// 非 nil 的 *rand.Rand 不能被多个 goroutine 共享，调用方若共享必须自行加锁。
func BuildChromeAcceptLanguageForCountry(country string, extra int, rng *rand.Rand) (string, error) {
	if extra < 0 {
		return "", &InvalidExtraLanguageCountError{Extra: extra}
	}
	primary, err := WebAcceptLanguageForCountry(country)
	if err != nil {
		return "", err
	}
	prefs, err := sampleChromeLanguagePrefs(primary, extra, rng)
	if err != nil {
		return "", err
	}
	return BuildChromeAcceptLanguage(prefs), nil
}

// chromeLanguageBase 取标签的语言族名。
// 输入 tag：已 trim 的 BCP 47 / Chrome data-code（如 zh-CN、zh-Hans-CN、ja）。
// 返回：第一个 '-' 之前的段（zh-Hans-CN → zh）；没有 '-' 则原样返回。
// 不能用「最后一个 '-'」或丢掉 script——Chromium 前瞻用的就是这一段。
func chromeLanguageBase(tag string) string {
	if i := strings.IndexByte(tag, '-'); i > 0 {
		return tag[:i]
	}
	return tag
}

// expandChromeLanguageList 复刻 Chromium ExpandLanguageList。
// 输入 tags：按偏好排序的 data-code，允许空白元素（丢掉），不改原切片。
// 返回：去重后的展开列表（zh-CN → zh-CN,zh）；输入全空返回长度为 0 的切片。
// 只看紧邻下一项是否同族：下一项同族就先不补基础语言（en-US,en-GB → 在 en-GB 后才补 en）；
// 远处再出现同族地区码不会回头重排。x-* / i-* 是私有/祖父标签，不补基础语言。
func expandChromeLanguageList(tags []string) []string {
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			cleaned = append(cleaned, tag)
		}
	}
	seen := make(map[string]struct{}, len(cleaned)*2)
	out := make([]string, 0, len(cleaned)*2)
	add := func(tag string) {
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	for i, tag := range cleaned {
		if _, ok := seen[tag]; ok {
			continue
		}
		add(tag)
		base := chromeLanguageBase(tag)
		if base == "x" || base == "i" {
			continue
		}
		if i+1 < len(cleaned) && chromeLanguageBase(cleaned[i+1]) == base {
			continue
		}
		add(base)
	}
	return out
}

// formatChromeAcceptLanguageQ 给已展开的标签挂 q 值。
// 输入 tags：expandChromeLanguageList 的输出，顺序即偏好。
// 返回：Accept-Language 字符串；空切片返回 ""。
// 用整数算 q，避免 0.1 浮点漂成 0.099…。第一项隐含 q=1.0 不写；到 0.1 后不再下降。
func formatChromeAcceptLanguageQ(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	for i, tag := range tags {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(tag)
		if i == 0 {
			continue
		}
		q := 10 - i
		if q < 1 {
			q = 1
		}
		b.WriteString(";q=0.")
		b.WriteByte(byte('0' + q))
	}
	return b.String()
}

// uniqueSortedChromeTags 取出 Web 表里所有不重复的 data-code。
// 输入：无（读 package 级 countryToWebAcceptTag，不改表）。
// 返回：升序切片副本。
// 表是 map，遍历顺序不稳；排序后同一 seed 的 Perm 才能复现。不缓存，避免测试或并发改到共享切片。
func uniqueSortedChromeTags() []string {
	seen := make(map[string]struct{}, len(countryToWebAcceptTag))
	for _, tag := range countryToWebAcceptTag {
		seen[tag] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

// sampleChromeLanguagePrefs 抽出「主码 + extra 个原始 Chrome 项」。
// 输入 primary：本国 data-code；extra：再抽几个（≥0）；rng：nil 用全局 Perm。
// 返回：prefs[0]==primary，其后 extra 个不与主码展开项重复的表内码；出错返回 (nil, err)。
// extra<0 → *InvalidExtraLanguageCountError；extra 大于剩余候选 → *ExtraLanguageCountExceedsPoolError。
// extra 数的是抽样前的 data-code，不是展开后的 header 项数；抽完不按语言族再排，
// 否则会把 Chromium 的前瞻顺序改掉。主码展开后已出现的精确码（zh-CN/zh）从池里去掉。
func sampleChromeLanguagePrefs(primary string, extra int, rng *rand.Rand) ([]string, error) {
	if extra < 0 {
		return nil, &InvalidExtraLanguageCountError{Extra: extra}
	}
	excluded := make(map[string]struct{})
	for _, tag := range expandChromeLanguageList([]string{primary}) {
		excluded[tag] = struct{}{}
	}
	pool := make([]string, 0, len(countryToWebAcceptTag))
	for _, tag := range uniqueSortedChromeTags() {
		if _, skip := excluded[tag]; !skip {
			pool = append(pool, tag)
		}
	}
	if extra > len(pool) {
		return nil, &ExtraLanguageCountExceedsPoolError{Extra: extra, Available: len(pool)}
	}
	prefs := make([]string, 0, 1+extra)
	prefs = append(prefs, primary)
	if extra == 0 {
		return prefs, nil
	}
	var perm []int
	if rng == nil {
		perm = rand.Perm(len(pool))
	} else {
		perm = rng.Perm(len(pool))
	}
	for i := 0; i < extra; i++ {
		prefs = append(prefs, pool[perm[i]])
	}
	return prefs, nil
}
