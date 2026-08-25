package geo

// countryToIANATimezone 按 ISO 3166-1 alpha-2 国家代码给出 IANA 时区【名称】。
//
// 与同目录 countryToTimezoneOffset 的分工：那张表给偏移秒数，供协议层填
// timezone_offset 字段；本表给名称，供浏览器层的 CDP Emulation.setTimezoneOverride
// 使用——该命令只接受 IANA 名称，不接受偏移。
//
// **不要用 Etc/GMT±N 兜底**：那类固定偏移时区确实合法，但真实用户机器上
// Intl.DateTimeFormat().resolvedOptions().timeZone 永远不会返回它，填了反而成为特征。
// 查不到就回退（见 IANATimezoneForCountry 的返回值语义），由调用方决定是告警还是放弃覆盖。
//
// 多时区国家取最具代表性的城市，与 countryToTimezoneOffset 的选取保持一致：
// US→纽约、AU→悉尼、CA→多伦多、RU→莫斯科、BR→圣保罗、MX→墨西哥城、ID→雅加达。
//
// 数据基于 IANA tzdata 2024。与 countryToTimezoneOffset 不同，本表【不要求】覆盖全部国家：
// 只收注册/养号业务实际会用到的出口国，缺了就补。
//
// **维护规约**：本表的每个 key 必须同时存在于 countryToTimezoneOffset，
// 由 TestIANATableKeysAreKnownCountries 守护——防止写错国家码后静默失效。
var countryToIANATimezone = map[string]string{
	// 北美
	"US": "America/New_York",
	"CA": "America/Toronto",
	"MX": "America/Mexico_City",

	// 南美
	"BR": "America/Sao_Paulo",
	"AR": "America/Argentina/Buenos_Aires",
	"CL": "America/Santiago",
	"CO": "America/Bogota",
	"PE": "America/Lima",
	"VE": "America/Caracas",
	"EC": "America/Guayaquil",
	"BO": "America/La_Paz",
	"PY": "America/Asuncion",
	"UY": "America/Montevideo",

	// 西欧 / 北欧
	"GB": "Europe/London",
	"IE": "Europe/Dublin",
	"FR": "Europe/Paris",
	"DE": "Europe/Berlin",
	"NL": "Europe/Amsterdam",
	"BE": "Europe/Brussels",
	"ES": "Europe/Madrid",
	"PT": "Europe/Lisbon",
	"IT": "Europe/Rome",
	"CH": "Europe/Zurich",
	"AT": "Europe/Vienna",
	"SE": "Europe/Stockholm",
	"NO": "Europe/Oslo",
	"DK": "Europe/Copenhagen",
	"FI": "Europe/Helsinki",

	// 中东欧 / 东欧
	"PL": "Europe/Warsaw",
	"CZ": "Europe/Prague",
	"HU": "Europe/Budapest",
	"RO": "Europe/Bucharest",
	"BG": "Europe/Sofia",
	"GR": "Europe/Athens",
	"UA": "Europe/Kyiv",
	"RU": "Europe/Moscow",
	"TR": "Europe/Istanbul",

	// 亚洲
	"IN": "Asia/Kolkata",
	"PK": "Asia/Karachi",
	"BD": "Asia/Dhaka",
	"LK": "Asia/Colombo",
	"NP": "Asia/Kathmandu",
	"ID": "Asia/Jakarta",
	"MY": "Asia/Kuala_Lumpur",
	"SG": "Asia/Singapore",
	"TH": "Asia/Bangkok",
	"VN": "Asia/Ho_Chi_Minh",
	"PH": "Asia/Manila",
	"KH": "Asia/Phnom_Penh",
	"MM": "Asia/Yangon",
	"JP": "Asia/Tokyo",
	"KR": "Asia/Seoul",
	"CN": "Asia/Shanghai",
	"HK": "Asia/Hong_Kong",
	"TW": "Asia/Taipei",
	"MO": "Asia/Macau",

	// 中东
	"AE": "Asia/Dubai",
	"SA": "Asia/Riyadh",
	"IL": "Asia/Jerusalem",
	"IQ": "Asia/Baghdad",
	"JO": "Asia/Amman",
	"KW": "Asia/Kuwait",
	"QA": "Asia/Qatar",
	"LB": "Asia/Beirut",

	// 非洲
	"NG": "Africa/Lagos",
	"EG": "Africa/Cairo",
	"ZA": "Africa/Johannesburg",
	"KE": "Africa/Nairobi",
	"GH": "Africa/Accra",
	"MA": "Africa/Casablanca",
	"DZ": "Africa/Algiers",
	"TN": "Africa/Tunis",
	"ET": "Africa/Addis_Ababa",
	"TZ": "Africa/Dar_es_Salaam",
	"UG": "Africa/Kampala",

	// 大洋洲
	"AU": "Australia/Sydney",
	"NZ": "Pacific/Auckland",
}

// IANATimezoneForCountry 按国家代码返回 IANA 时区名。
// 第二个返回值为 false 表示本表未收录——调用方应当放弃时区覆盖并告警，
// 而不是自己编一个（编错了比不设更容易暴露）。
func IANATimezoneForCountry(country string) (string, bool) {
	if country == "" {
		return "", false
	}
	tz, ok := countryToIANATimezone[normalizeCountryKey(country)]
	return tz, ok
}
