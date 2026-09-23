package localemobile

// mapped_locale.go —— 国家码 → X-IG-Mapped-Locale。
// 依据 IG Android 429 LX/03uv.A01：取 App Locale 的 toLanguageTag 首段语言，依次用
// lang-script-country、lang-script、lang-country 查 LX/03uv.A00 的 19 条特殊标签，
// 未命中再用 getLanguage() 查内联的 102 条语言表，都没有返回 en_US。
// 因此 en 只有 en-GB 保留，其余英语区都是 en_US；es 默认 es_LA（ES 为 es_ES）；
// pt 默认 pt_BR（PT 为 pt_PT）；MO 的 zh-Hant-MO 退到 zh-Hant → zh_TW。
// 值按上述规则算过一次，写进字面量；运行期不再判断。

// countryToMappedLocale 国家码 → Meta 内部 locale。例：CN → zh_CN，MX → es_LA，AU → en_US。
var countryToMappedLocale = map[string]string{
	// 英语主导（北美、UK、印太、撒哈拉以南非洲英语主语言国家、加勒比英语小岛、印度洋英语）
	"US": "en_US", "GB": "en_GB", "CA": "en_US", "AU": "en_US", "NZ": "en_US",
	"IE": "en_US", "IN": "en_US", "SG": "en_US", "PH": "en_US", "ZA": "en_US",
	"NG": "en_US", "GH": "en_US", "PK": "en_US", "JM": "en_US", "TT": "en_US",
	"MT": "en_US",
	"BS": "en_US", "AI": "en_US", "BM": "en_US", "DM": "en_US", "GY": "en_US", "VC": "en_US",
	"MU": "en_US", "MV": "en_US", "MW": "en_US", "NA": "en_US", "SC": "en_US", "SL": "en_US",
	"ZW": "en_US",
	// 加勒比英语 / 北美附属
	"AG": "en_US", "BB": "en_US", "BZ": "en_US", "GD": "en_US",
	"KN": "en_US", "KY": "en_US", "LC": "en_US", "TC": "en_US",
	"VG": "en_US", "VI": "en_US",
	// 大洋洲 / 太平洋英语
	"FJ": "en_US", "GU": "en_US", "PG": "en_US", "PW": "en_US", "WS": "en_US",
	// 不列颠群岛附属
	"GG": "en_US", "IM": "en_US", "JE": "en_US",
	// 直布罗陀
	"GI": "en_US",
	// 撒哈拉以南非洲英语扩展
	"BW": "en_US", "GM": "en_US", "LR": "en_US", "LS": "en_US",
	"SS": "en_US", "SZ": "en_US", "ZM": "en_US",

	// 东亚
	"CN": "zh_CN", "TW": "zh_TW", "HK": "zh_HK", "MO": "zh_TW",
	"JP": "ja_JP", "KR": "ko_KR", "MN": "mn_MN",

	// 东南亚
	"ID": "id_ID", "TH": "th_TH", "VN": "vi_VN", "MY": "ms_MY",
	"KH": "km_KH", "LA": "lo_LA", "MM": "my_MM", "BN": "ms_MY",

	// 南亚（IN 已在英语区）
	"BD": "bn_IN", "LK": "si_LK", "NP": "ne_NP", "AF": "ps_AF", "BT": "en_US",

	// 西欧
	"FR": "fr_FR", "DE": "de_DE", "IT": "it_IT", "ES": "es_ES", "PT": "pt_PT",
	"NL": "nl_NL", "BE": "nl_NL", "LU": "fr_FR", "CH": "de_DE", "AT": "de_DE",
	"LI": "de_DE", "MC": "fr_FR", "AD": "ca_ES", "SM": "it_IT", "VA": "it_IT",
	"CW": "nl_NL", "AW": "nl_NL", "SX": "nl_NL", "BQ": "nl_NL",
	// 荷语苏里南
	"SR": "nl_NL",

	// 拉美
	"BR": "pt_BR", "MX": "es_LA", "AR": "es_LA", "CL": "es_LA", "CO": "es_LA",
	"PE": "es_LA", "VE": "es_LA", "EC": "es_LA", "BO": "es_LA", "UY": "es_LA",
	"PY": "es_LA", "CR": "es_LA", "DO": "es_LA", "GT": "es_LA", "HN": "es_LA",
	"NI": "es_LA", "PA": "es_LA", "SV": "es_LA", "CU": "es_LA", "PR": "es_LA",

	// 法属海外（加勒比 / 南美 / 北大西洋 / 太平洋）
	"GF": "fr_FR", "GP": "fr_FR", "HT": "fr_FR", "MF": "fr_FR", "MQ": "fr_FR",
	"PM": "fr_FR", "NC": "fr_FR", "PF": "fr_FR", "WF": "fr_FR",

	// 北欧
	"DK": "da_DK", "SE": "sv_SE", "NO": "nb_NO", "FI": "fi_FI",
	"IS": "is_IS", "FO": "fo_FO", "GL": "en_US",

	// 东欧 / 巴尔干 / 波罗的海
	"RU": "ru_RU", "UA": "uk_UA", "BY": "be_BY", "PL": "pl_PL", "CZ": "cs_CZ",
	"SK": "sk_SK", "HU": "hu_HU", "RO": "ro_RO", "MD": "ro_RO", "BG": "bg_BG",
	"GR": "el_GR", "CY": "el_GR", "HR": "hr_HR", "RS": "sr_RS", "SI": "sl_SI",
	"BA": "bs_BA", "MK": "mk_MK", "AL": "sq_AL", "ME": "sr_RS", "XK": "sq_AL",
	"LT": "lt_LT", "LV": "lv_LV", "EE": "et_EE",

	// 中东 / 高加索
	"TR": "tr_TR", "IL": "he_IL", "IR": "fa_IR",
	"SA": "ar_AR", "AE": "ar_AR", "EG": "ar_AR", "MA": "ar_AR", "DZ": "ar_AR",
	"TN": "ar_AR", "JO": "ar_AR", "LB": "ar_AR", "KW": "ar_AR", "QA": "ar_AR",
	"BH": "ar_AR", "OM": "ar_AR", "IQ": "ar_AR", "YE": "ar_AR", "LY": "ar_AR",
	"SD": "ar_AR", "SY": "ar_AR", "PS": "ar_AR",
	"AM": "hy_AM", "AZ": "az_AZ", "GE": "ka_GE",
	// 毛里塔尼亚阿语
	"MR": "ar_AR",

	// 中亚
	"KZ": "kk_KZ", "UZ": "uz_UZ", "KG": "ky_KG", "TJ": "tg_TJ", "TM": "tk_TM",

	// 撒哈拉以南非洲（英语国家已上面列）
	"KE": "sw_KE", "TZ": "sw_KE", "UG": "sw_KE", "ET": "am_ET", "RW": "rw_RW",
	"SN": "fr_FR", "CI": "fr_FR", "CM": "fr_FR", "MG": "mg_MG",
	"BF": "fr_FR", "BJ": "fr_FR", "CD": "fr_FR", "CG": "fr_FR", "GN": "fr_FR", "ML": "fr_FR",
	"AO": "pt_BR", "CV": "pt_BR", "MZ": "pt_BR", "ST": "pt_BR",
	// 法语非洲扩展 / 西葡非洲 / 索马里语 / 印度洋法语
	"BI": "fr_FR", "CF": "fr_FR", "DJ": "fr_FR", "GA": "fr_FR",
	"NE": "fr_FR", "TD": "fr_FR", "TG": "fr_FR",
	"GQ": "es_LA", "GW": "pt_BR",
	"SO": "so_SO",
	"KM": "fr_FR", "RE": "fr_FR", "YT": "fr_FR",
}

// MappedLocaleForCountry 按国家代码查 Meta 内部 mapped locale。
// 输入 country：ISO 3166-1 alpha-2，大小写不敏感，自动 trim。
// 返回：表值；空输入或未命中返回 ("", *geo.UnknownCountryError)，请用 errors.As 判定。
// 例："cn" → ("zh_CN", nil)；"MX" → ("es_LA", nil)；"ID" → ("id_ID", nil)；"BT" → ("en_US", nil)。
func MappedLocaleForCountry(country string) (string, error) {
	return lookupCountry(countryToMappedLocale, country)
}
