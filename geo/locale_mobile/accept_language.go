package localemobile

// accept_language.go —— 国家码 → Accept-Language。
// 依据 IG Android 429 LX/03ix.A00：Locale.getDefault() 经 LX/03fj.A00 格式化为
// 「语言-国家」（in/iw/ji 换回 id/he/yi，不带 script），不等于 Locale.US 时追加 ", en-US"。
// 未设 App 语言偏好时默认 Locale 就是系统 Locale，所以按国家表的系统 Locale 算出。
// 值按上述规则算过一次，写进字面量；运行期不再判断。

// countryToAcceptLanguage 国家码 → Accept-Language。US 是 en-US，其余都以 ", en-US" 结尾。
var countryToAcceptLanguage = map[string]string{ //nolint:gosec // G101 误报：值是语言标签，不是凭据
	// 英语主导（北美、UK、印太、撒哈拉以南非洲英语主语言国家、加勒比英语小岛、印度洋英语）
	"US": "en-US", "GB": "en-GB, en-US", "CA": "en-CA, en-US", "AU": "en-AU, en-US", "NZ": "en-NZ, en-US",
	"IE": "en-IE, en-US", "IN": "en-IN, en-US", "SG": "en-SG, en-US", "PH": "en-PH, en-US",
	"ZA": "en-ZA, en-US",
	"NG": "en-NG, en-US", "GH": "en-GH, en-US", "PK": "en-PK, en-US", "JM": "en-JM, en-US",
	"TT": "en-TT, en-US",
	"MT": "en-MT, en-US",
	"BS": "en-BS, en-US", "AI": "en-AI, en-US", "BM": "en-BM, en-US", "DM": "en-DM, en-US",
	"GY": "en-GY, en-US", "VC": "en-VC, en-US",
	"MU": "en-MU, en-US", "MV": "en-MV, en-US", "MW": "en-MW, en-US", "NA": "en-NA, en-US",
	"SC": "en-SC, en-US", "SL": "en-SL, en-US",
	"ZW": "en-ZW, en-US",
	// 加勒比英语 / 北美附属
	"AG": "en-AG, en-US", "BB": "en-BB, en-US", "BZ": "en-BZ, en-US", "GD": "en-GD, en-US",
	"KN": "en-KN, en-US", "KY": "en-KY, en-US", "LC": "en-LC, en-US", "TC": "en-TC, en-US",
	"VG": "en-VG, en-US", "VI": "en-VI, en-US",
	// 大洋洲 / 太平洋英语
	"FJ": "en-FJ, en-US", "GU": "en-GU, en-US", "PG": "en-PG, en-US", "PW": "en-PW, en-US",
	"WS": "en-WS, en-US",
	// 不列颠群岛附属
	"GG": "en-GG, en-US", "IM": "en-IM, en-US", "JE": "en-JE, en-US",
	// 直布罗陀
	"GI": "en-GI, en-US",
	// 撒哈拉以南非洲英语扩展
	"BW": "en-BW, en-US", "GM": "en-GM, en-US", "LR": "en-LR, en-US", "LS": "en-LS, en-US",
	"SS": "en-SS, en-US", "SZ": "en-SZ, en-US", "ZM": "en-ZM, en-US",

	// 东亚
	"CN": "zh-CN, en-US", "TW": "zh-TW, en-US", "HK": "zh-HK, en-US", "MO": "zh-MO, en-US",
	"JP": "ja-JP, en-US", "KR": "ko-KR, en-US", "MN": "mn-MN, en-US",

	// 东南亚
	"ID": "id-ID, en-US", "TH": "th-TH, en-US", "VN": "vi-VN, en-US", "MY": "ms-MY, en-US",
	"KH": "km-KH, en-US", "LA": "lo-LA, en-US", "MM": "my-MM, en-US", "BN": "ms-BN, en-US",

	// 南亚（IN 已在英语区）
	"BD": "bn-BD, en-US", "LK": "si-LK, en-US", "NP": "ne-NP, en-US", "AF": "ps-AF, en-US",
	"BT": "dz-BT, en-US",

	// 西欧
	"FR": "fr-FR, en-US", "DE": "de-DE, en-US", "IT": "it-IT, en-US", "ES": "es-ES, en-US",
	"PT": "pt-PT, en-US",
	"NL": "nl-NL, en-US", "BE": "nl-BE, en-US", "LU": "fr-LU, en-US", "CH": "de-CH, en-US",
	"AT": "de-AT, en-US",
	"LI": "de-LI, en-US", "MC": "fr-MC, en-US", "AD": "ca-AD, en-US", "SM": "it-SM, en-US",
	"VA": "it-VA, en-US",
	"CW": "nl-CW, en-US", "AW": "nl-AW, en-US", "SX": "nl-SX, en-US", "BQ": "nl-BQ, en-US",
	// 荷语苏里南
	"SR": "nl-SR, en-US",

	// 拉美
	"BR": "pt-BR, en-US", "MX": "es-MX, en-US", "AR": "es-AR, en-US", "CL": "es-CL, en-US",
	"CO": "es-CO, en-US",
	"PE": "es-PE, en-US", "VE": "es-VE, en-US", "EC": "es-EC, en-US", "BO": "es-BO, en-US",
	"UY": "es-UY, en-US",
	"PY": "es-PY, en-US", "CR": "es-CR, en-US", "DO": "es-DO, en-US", "GT": "es-GT, en-US",
	"HN": "es-HN, en-US",
	"NI": "es-NI, en-US", "PA": "es-PA, en-US", "SV": "es-SV, en-US", "CU": "es-CU, en-US",
	"PR": "es-PR, en-US",

	// 法属海外（加勒比 / 南美 / 北大西洋 / 太平洋）
	"GF": "fr-GF, en-US", "GP": "fr-GP, en-US", "HT": "fr-HT, en-US", "MF": "fr-MF, en-US",
	"MQ": "fr-MQ, en-US",
	"PM": "fr-PM, en-US", "NC": "fr-NC, en-US", "PF": "fr-PF, en-US", "WF": "fr-WF, en-US",

	// 北欧
	"DK": "da-DK, en-US", "SE": "sv-SE, en-US", "NO": "nb-NO, en-US", "FI": "fi-FI, en-US",
	"IS": "is-IS, en-US", "FO": "fo-FO, en-US", "GL": "kl-GL, en-US",

	// 东欧 / 巴尔干 / 波罗的海
	"RU": "ru-RU, en-US", "UA": "uk-UA, en-US", "BY": "be-BY, en-US", "PL": "pl-PL, en-US",
	"CZ": "cs-CZ, en-US",
	"SK": "sk-SK, en-US", "HU": "hu-HU, en-US", "RO": "ro-RO, en-US", "MD": "ro-MD, en-US",
	"BG": "bg-BG, en-US",
	"GR": "el-GR, en-US", "CY": "el-CY, en-US", "HR": "hr-HR, en-US", "RS": "sr-RS, en-US",
	"SI": "sl-SI, en-US",
	"BA": "bs-BA, en-US", "MK": "mk-MK, en-US", "AL": "sq-AL, en-US", "ME": "sr-ME, en-US",
	"XK": "sq-XK, en-US",
	"LT": "lt-LT, en-US", "LV": "lv-LV, en-US", "EE": "et-EE, en-US",

	// 中东 / 高加索
	"TR": "tr-TR, en-US", "IL": "he-IL, en-US", "IR": "fa-IR, en-US",
	"SA": "ar-SA, en-US", "AE": "ar-AE, en-US", "EG": "ar-EG, en-US", "MA": "ar-MA, en-US",
	"DZ": "ar-DZ, en-US",
	"TN": "ar-TN, en-US", "JO": "ar-JO, en-US", "LB": "ar-LB, en-US", "KW": "ar-KW, en-US",
	"QA": "ar-QA, en-US",
	"BH": "ar-BH, en-US", "OM": "ar-OM, en-US", "IQ": "ar-IQ, en-US", "YE": "ar-YE, en-US",
	"LY": "ar-LY, en-US",
	"SD": "ar-SD, en-US", "SY": "ar-SY, en-US", "PS": "ar-PS, en-US",
	"AM": "hy-AM, en-US", "AZ": "az-AZ, en-US", "GE": "ka-GE, en-US",
	// 毛里塔尼亚阿语
	"MR": "ar-MR, en-US",

	// 中亚
	"KZ": "kk-KZ, en-US", "UZ": "uz-UZ, en-US", "KG": "ky-KG, en-US", "TJ": "tg-TJ, en-US",
	"TM": "tk-TM, en-US",

	// 撒哈拉以南非洲（英语国家已上面列）
	"KE": "sw-KE, en-US", "TZ": "sw-TZ, en-US", "UG": "sw-UG, en-US", "ET": "am-ET, en-US",
	"RW": "rw-RW, en-US",
	"SN": "fr-SN, en-US", "CI": "fr-CI, en-US", "CM": "fr-CM, en-US", "MG": "mg-MG, en-US",
	"BF": "fr-BF, en-US", "BJ": "fr-BJ, en-US", "CD": "fr-CD, en-US", "CG": "fr-CG, en-US",
	"GN": "fr-GN, en-US", "ML": "fr-ML, en-US",
	"AO": "pt-AO, en-US", "CV": "pt-CV, en-US", "MZ": "pt-MZ, en-US", "ST": "pt-ST, en-US",
	// 法语非洲扩展 / 西葡非洲 / 索马里语 / 印度洋法语
	"BI": "fr-BI, en-US", "CF": "fr-CF, en-US", "DJ": "fr-DJ, en-US", "GA": "fr-GA, en-US",
	"NE": "fr-NE, en-US", "TD": "fr-TD, en-US", "TG": "fr-TG, en-US",
	"GQ": "es-GQ, en-US", "GW": "pt-GW, en-US",
	"SO": "so-SO, en-US",
	"KM": "fr-KM, en-US", "RE": "fr-RE, en-US", "YT": "fr-YT, en-US",
}

// AcceptLanguageForCountry 按国家代码查 Android 端 Accept-Language。
// 输入 country：ISO 3166-1 alpha-2，大小写不敏感，自动 trim。
// 返回：表值；空输入或未命中返回 ("", *geo.UnknownCountryError)，请用 errors.As 判定。
// 例："cn" → ("zh-CN, en-US", nil)；"US" → ("en-US", nil)；"ID" → ("id-ID, en-US", nil)。
func AcceptLanguageForCountry(country string) (string, error) {
	return lookupCountry(countryToAcceptLanguage, country)
}
