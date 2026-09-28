package localemobile

import "github.com/japansms40-web/gohttpkit/geo"

// device_locale.go —— 国家码 → X-IG-Device-Locale。
// 依据 IG Android 429 LX/01pH.A03：Resources.getSystem().getConfiguration().locale.toString()，
// 即系统语言列表第一项。App 内切换语言不影响它。
// toString 规则与 app_locale.go 相同；本表假设未设 App 语言偏好，此时 device locale 与 app locale 相同，
// 所以直接复用 countryToAppLocale，不再重复一份字面量。需要单独调整时再恢复成独立字面量。

// countryToDeviceLocale 国家码 → 系统 Locale.toString()。例：CN → zh_CN_#Hans。
// 与 countryToAppLocale 共用同一个 map，两者都只读。
var countryToDeviceLocale = countryToAppLocale

// DeviceLocaleForCountry 按国家代码查系统 Locale.toString()。
// 输入 country：ISO 3166-1 alpha-2，大小写不敏感，自动 trim。
// 返回：表值；空输入或未命中返回 ("", *geo.UnknownCountryError)，请用 errors.As 判定。
// 例："cn" → ("zh_CN_#Hans", nil)；"IL" → ("iw_IL", nil)；"RS" → ("sr_RS_#Cyrl", nil)。
func DeviceLocaleForCountry(country string) (string, error) {
	return geo.LookupCountry(countryToDeviceLocale, country)
}
