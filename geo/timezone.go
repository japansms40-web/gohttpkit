package geo

// timezone.go —— 国家码 → 冬令时偏移秒数大表。
// 独立成文件：与 locale 表分开维护，keyset 由测试对齐，避免「加了语言忘了时区」。

// countryToTimezoneOffset 按 ISO 3166-1 alpha-2 国家代码派生 IANA 主时区的"冬令时（标准时间）"
// 偏移秒数。东半球正、西半球负。**不处理 DST**：欧洲国家夏令时偏移 +1h 不在此表表达，
// 如需精准 DST 行为请通过 Config.TimezoneOffset 显式覆盖。
//
// 多时区国家选最具代表性城市：US→纽约 EST、AU→悉尼 AEST、CA→多伦多 EST、
// RU→莫斯科 MSK、BR→圣保罗、MX→墨西哥城。
//
// 数据基于 IANA tzdata 2024 版本。已实行永久标准时间的国家（如 RU/MX/BR/AZ 等）
// 直接使用永久偏移；NZ/AU/CL/PY 等南半球国家使用其冬令时（北半球夏季）偏移。
//
// **维护规约**：本表 keyset 必须与 countryToMobileLocale 完全对齐——任一边新增 country
// 都必须同步加，由 TestTimezoneTableKeysetMatchesLocaleTable 守护。
var countryToTimezoneOffset = map[string]int{
	"AD": 3600, "AE": 14400, "AF": 16200, "AG": -14400, "AI": -14400, "AL": 3600,
	"AM": 14400, "AO": 3600, "AR": -10800, "AT": 3600, "AU": 36000, "AW": -14400,
	"AZ": 14400, "BA": 3600, "BB": -14400, "BD": 21600, "BE": 3600, "BF": 0,
	"BG": 7200, "BH": 10800, "BI": 7200, "BJ": 3600, "BM": -14400, "BN": 28800,
	"BO": -14400, "BQ": -14400, "BR": -10800, "BS": -18000, "BT": 21600, "BW": 7200,
	"BY": 10800, "BZ": -21600, "CA": -18000, "CD": 3600, "CF": 3600, "CG": 3600,
	"CH": 3600, "CI": 0, "CL": -14400, "CM": 3600, "CN": 28800, "CO": -18000,
	"CR": -21600, "CU": -18000, "CV": -3600, "CW": -14400, "CY": 7200, "CZ": 3600,
	"DE": 3600, "DJ": 10800, "DK": 3600, "DM": -14400, "DO": -14400, "DZ": 3600,
	"EC": -18000, "EE": 7200, "EG": 7200, "ES": 3600, "ET": 10800, "FI": 7200,
	"FJ": 43200, "FO": 0, "FR": 3600, "GA": 3600, "GB": 0, "GD": -14400,
	"GE": 14400, "GF": -10800, "GG": 0, "GH": 0, "GI": 3600, "GL": -7200,
	"GM": 0, "GN": 0, "GP": -14400, "GQ": 3600, "GR": 7200, "GT": -21600,
	"GU": 36000, "GW": 0, "GY": -14400, "HK": 28800, "HN": -21600, "HR": 3600,
	"HT": -18000, "HU": 3600, "ID": 25200, "IE": 0, "IL": 7200, "IM": 0,
	"IN": 19800, "IQ": 10800, "IR": 12600, "IS": 0, "IT": 3600, "JE": 0,
	"JM": -18000, "JO": 10800, "JP": 32400, "KE": 10800, "KG": 21600, "KH": 25200,
	"KM": 10800, "KN": -14400, "KR": 32400, "KW": 10800, "KY": -18000, "KZ": 18000,
	"LA": 25200, "LB": 7200, "LC": -14400, "LI": 3600, "LK": 19800, "LR": 0,
	"LS": 7200, "LT": 7200, "LU": 3600, "LV": 7200, "LY": 7200, "MA": 3600,
	"MC": 3600, "MD": 7200, "ME": 3600, "MF": -14400, "MG": 10800, "MK": 3600,
	"ML": 0, "MM": 23400, "MN": 28800, "MO": 28800, "MQ": -14400, "MR": 0,
	"MT": 3600, "MU": 14400, "MV": 18000, "MW": 7200, "MX": -21600, "MY": 28800,
	"MZ": 7200, "NA": 7200, "NC": 39600, "NE": 3600, "NG": 3600, "NI": -21600,
	"NL": 3600, "NO": 3600, "NP": 20700, "NZ": 43200, "OM": 14400, "PA": -18000,
	"PE": -18000, "PF": -36000, "PG": 36000, "PH": 28800, "PK": 18000, "PL": 3600,
	"PM": -10800, "PR": -14400, "PS": 7200, "PT": 0, "PW": 32400, "PY": -14400,
	"QA": 10800, "RE": 14400, "RO": 7200, "RS": 3600, "RU": 10800, "RW": 7200,
	"SA": 10800, "SC": 14400, "SD": 7200, "SE": 3600, "SG": 28800, "SI": 3600,
	"SK": 3600, "SL": 0, "SM": 3600, "SN": 0, "SO": 10800, "SR": -10800,
	"SS": 7200, "ST": 0, "SV": -21600, "SX": -14400, "SY": 10800, "SZ": 7200,
	"TC": -18000, "TD": 3600, "TG": 0, "TH": 25200, "TJ": 18000, "TM": 18000,
	"TN": 3600, "TR": 10800, "TT": -14400, "TW": 28800, "TZ": 10800, "UA": 7200,
	"UG": 10800, "US": -18000, "UY": -10800, "UZ": 18000, "VA": 3600, "VC": -14400,
	"VE": -14400, "VG": -14400, "VI": -14400, "VN": 25200, "WF": 43200, "WS": 46800,
	"XK": 3600, "YE": 10800, "YT": 10800, "ZA": 7200, "ZM": 7200, "ZW": 7200,
}

// TimezoneOffsetForCountry 按国家代码查冬令时偏移秒数。
// 输入 country：ISO 3166-1 alpha-2，大小写不敏感，自动 trim。
// 返回：命中是 (offset, nil)；空输入或未命中是 (0, *UnknownCountryError)。
// 例："jp" → (32400, nil)；"gb" → (0, nil)；"" → (0, *UnknownCountryError{Country:""})。
// 0 是合法 offset（GB 等 GMT+0），必须用 error 区分，不要用 offset==0 当失败。
func TimezoneOffsetForCountry(country string) (int, error) {
	key := normalizeCountryKey(country)
	if key == "" {
		return 0, &UnknownCountryError{Country: ""}
	}
	off, ok := countryToTimezoneOffset[key]
	if !ok {
		return 0, &UnknownCountryError{Country: key}
	}
	return off, nil
}
