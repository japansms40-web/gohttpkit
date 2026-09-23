package localemobile

// app_locale.go —— 国家码 → X-IG-App-Locale。
// 依据 IG Android 429 LX/01pH.A02：应用 Resources 的 Locale.toString()。未设 App 语言偏好
// （fb_language_locale）时，LX/01pH.A04 把系统 Locale 写进应用 Resources，所以与 device locale 相同。
// toString 规则：语言用 Android 旧码（in_ID、iw_IL），有 script 时接 "_#Script"；
// script 取 AOSP supported_locales 中该国可选项（CN Hans，TW/HK/MO Hant，RS Cyrl，ME/BA/AZ/UZ Latn）。
// 值按上述规则算过一次，写进字面量；运行期不再判断。

// countryToAppLocale 国家码 → 应用 Locale.toString()。例：CN → zh_CN_#Hans。
var countryToAppLocale = map[string]string{
	// 英语主导（北美、UK、印太、撒哈拉以南非洲英语主语言国家、加勒比英语小岛、印度洋英语）
	"US": "en_US", "GB": "en_GB", "CA": "en_CA", "AU": "en_AU", "NZ": "en_NZ",
	"IE": "en_IE", "IN": "en_IN", "SG": "en_SG", "PH": "en_PH", "ZA": "en_ZA",
	"NG": "en_NG", "GH": "en_GH", "PK": "en_PK", "JM": "en_JM", "TT": "en_TT",
	"MT": "en_MT",
	"BS": "en_BS", "AI": "en_AI", "BM": "en_BM", "DM": "en_DM", "GY": "en_GY", "VC": "en_VC",
	"MU": "en_MU", "MV": "en_MV", "MW": "en_MW", "NA": "en_NA", "SC": "en_SC", "SL": "en_SL",
	"ZW": "en_ZW",
	// 加勒比英语 / 北美附属
	"AG": "en_AG", "BB": "en_BB", "BZ": "en_BZ", "GD": "en_GD",
	"KN": "en_KN", "KY": "en_KY", "LC": "en_LC", "TC": "en_TC",
	"VG": "en_VG", "VI": "en_VI",
	// 大洋洲 / 太平洋英语
	"FJ": "en_FJ", "GU": "en_GU", "PG": "en_PG", "PW": "en_PW", "WS": "en_WS",
	// 不列颠群岛附属
	"GG": "en_GG", "IM": "en_IM", "JE": "en_JE",
	// 直布罗陀
	"GI": "en_GI",
	// 撒哈拉以南非洲英语扩展
	"BW": "en_BW", "GM": "en_GM", "LR": "en_LR", "LS": "en_LS",
	"SS": "en_SS", "SZ": "en_SZ", "ZM": "en_ZM",

	// 东亚
	"CN": "zh_CN_#Hans", "TW": "zh_TW_#Hant", "HK": "zh_HK_#Hant", "MO": "zh_MO_#Hant",
	"JP": "ja_JP", "KR": "ko_KR", "MN": "mn_MN",

	// 东南亚
	"ID": "in_ID", "TH": "th_TH", "VN": "vi_VN", "MY": "ms_MY",
	"KH": "km_KH", "LA": "lo_LA", "MM": "my_MM", "BN": "ms_BN",

	// 南亚（IN 已在英语区）
	"BD": "bn_BD", "LK": "si_LK", "NP": "ne_NP", "AF": "ps_AF", "BT": "dz_BT",

	// 西欧
	"FR": "fr_FR", "DE": "de_DE", "IT": "it_IT", "ES": "es_ES", "PT": "pt_PT",
	"NL": "nl_NL", "BE": "nl_BE", "LU": "fr_LU", "CH": "de_CH", "AT": "de_AT",
	"LI": "de_LI", "MC": "fr_MC", "AD": "ca_AD", "SM": "it_SM", "VA": "it_VA",
	"CW": "nl_CW", "AW": "nl_AW", "SX": "nl_SX", "BQ": "nl_BQ",
	// 荷语苏里南
	"SR": "nl_SR",

	// 拉美
	"BR": "pt_BR", "MX": "es_MX", "AR": "es_AR", "CL": "es_CL", "CO": "es_CO",
	"PE": "es_PE", "VE": "es_VE", "EC": "es_EC", "BO": "es_BO", "UY": "es_UY",
	"PY": "es_PY", "CR": "es_CR", "DO": "es_DO", "GT": "es_GT", "HN": "es_HN",
	"NI": "es_NI", "PA": "es_PA", "SV": "es_SV", "CU": "es_CU", "PR": "es_PR",

	// 法属海外（加勒比 / 南美 / 北大西洋 / 太平洋）
	"GF": "fr_GF", "GP": "fr_GP", "HT": "fr_HT", "MF": "fr_MF", "MQ": "fr_MQ",
	"PM": "fr_PM", "NC": "fr_NC", "PF": "fr_PF", "WF": "fr_WF",

	// 北欧
	"DK": "da_DK", "SE": "sv_SE", "NO": "nb_NO", "FI": "fi_FI",
	"IS": "is_IS", "FO": "fo_FO", "GL": "kl_GL",

	// 东欧 / 巴尔干 / 波罗的海
	"RU": "ru_RU", "UA": "uk_UA", "BY": "be_BY", "PL": "pl_PL", "CZ": "cs_CZ",
	"SK": "sk_SK", "HU": "hu_HU", "RO": "ro_RO", "MD": "ro_MD", "BG": "bg_BG",
	"GR": "el_GR", "CY": "el_CY", "HR": "hr_HR", "RS": "sr_RS_#Cyrl", "SI": "sl_SI",
	"BA": "bs_BA_#Latn", "MK": "mk_MK", "AL": "sq_AL", "ME": "sr_ME_#Latn", "XK": "sq_XK",
	"LT": "lt_LT", "LV": "lv_LV", "EE": "et_EE",

	// 中东 / 高加索
	"TR": "tr_TR", "IL": "iw_IL", "IR": "fa_IR",
	"SA": "ar_SA", "AE": "ar_AE", "EG": "ar_EG", "MA": "ar_MA", "DZ": "ar_DZ",
	"TN": "ar_TN", "JO": "ar_JO", "LB": "ar_LB", "KW": "ar_KW", "QA": "ar_QA",
	"BH": "ar_BH", "OM": "ar_OM", "IQ": "ar_IQ", "YE": "ar_YE", "LY": "ar_LY",
	"SD": "ar_SD", "SY": "ar_SY", "PS": "ar_PS",
	"AM": "hy_AM", "AZ": "az_AZ_#Latn", "GE": "ka_GE",
	// 毛里塔尼亚阿语
	"MR": "ar_MR",

	// 中亚
	"KZ": "kk_KZ", "UZ": "uz_UZ_#Latn", "KG": "ky_KG", "TJ": "tg_TJ", "TM": "tk_TM",

	// 撒哈拉以南非洲（英语国家已上面列）
	"KE": "sw_KE", "TZ": "sw_TZ", "UG": "sw_UG", "ET": "am_ET", "RW": "rw_RW",
	"SN": "fr_SN", "CI": "fr_CI", "CM": "fr_CM", "MG": "mg_MG",
	"BF": "fr_BF", "BJ": "fr_BJ", "CD": "fr_CD", "CG": "fr_CG", "GN": "fr_GN", "ML": "fr_ML",
	"AO": "pt_AO", "CV": "pt_CV", "MZ": "pt_MZ", "ST": "pt_ST",
	// 法语非洲扩展 / 西葡非洲 / 索马里语 / 印度洋法语
	"BI": "fr_BI", "CF": "fr_CF", "DJ": "fr_DJ", "GA": "fr_GA",
	"NE": "fr_NE", "TD": "fr_TD", "TG": "fr_TG",
	"GQ": "es_GQ", "GW": "pt_GW",
	"SO": "so_SO",
	"KM": "fr_KM", "RE": "fr_RE", "YT": "fr_YT",
}

// AppLocaleForCountry 按国家代码查应用 Locale.toString()（未设 App 语言偏好的画像）。
// 输入 country：ISO 3166-1 alpha-2，大小写不敏感，自动 trim。
// 返回：表值；空输入或未命中返回 ("", *geo.UnknownCountryError)，请用 errors.As 判定。
// 例："cn" → ("zh_CN_#Hans", nil)；"ID" → ("in_ID", nil)；"MO" → ("zh_MO_#Hant", nil)。
func AppLocaleForCountry(country string) (string, error) {
	return lookupCountry(countryToAppLocale, country)
}
