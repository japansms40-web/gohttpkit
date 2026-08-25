package geo

import (
	"fmt"
	"strings"
)

// 精细化 Android locale 派生:
// 真机抓包(某主流社交 App Android 客户端 429 版本)显示
// locale 相关 5 个 header 各有不同格式,不能用同一个 "zh_CN" 字符串复用:
//
//   header                     格式             示例(zh_CN)
//   -----------------------    --------------   -----------------------------------------
//   accept-language            BCP47 列表       "zh-CN, en-US"
//   x-ig-app-locale            下划线 + 脚本    "zh_CN_#Hans"
//   x-ig-device-locale         同上             "zh_CN_#Hans"
//   x-ig-mapped-locale         下划线(裸)       "zh_CN"           ← 即原 MobileLocaleForCountry
//   x-ig-device-languages      JSON 对象        {"system_languages":"zh-CN, en-US","keyboard_language":"en-US"}
//
// 这些 helper 全部以 MobileLocaleForCountry 返回的下划线 locale 为输入。

// localeToScript ISO 15924 脚本码白名单(只列实测有脚本后缀的 locale)
// 大多数拉丁字母语言 Latn 是默认值,真机不带后缀
var localeToScript = map[string]string{
	"zh_CN": "Hans", // 简体中文(中国大陆)
	"zh_SG": "Hans", // 简体中文(新加坡)
	"zh_MY": "Hans", // 简体中文(马来西亚)
	"zh_HK": "Hant", // 繁体中文(香港)
	"zh_TW": "Hant", // 繁体中文(台湾)
	"zh_MO": "Hant", // 繁体中文(澳门)
}

// AndroidAcceptLanguage 派生 Accept-Language header 值
//
// 真机格式(某社交 App Android 429 抓包):"zh-CN, en-US"(BCP47 + 备选英语,空格分隔,不带 ;q 权重)
// 单语言策略:
//   - 主语言为 en(en_US / en_GB 等)→ 只返回 "en-US"(不重复)
//   - 主语言非 en → 返回 "{primary-bcp47}, en-US"(加英语备选)
//
// 接受 "zh_CN" 或 "zh-CN" 两种形式输入,内部统一转 BCP47(连字符)
func AndroidAcceptLanguage(locale string) string {
	bcp47 := toBCP47(locale)
	// 主语言判定（Android 侧大小写不敏感，与 Web 侧 == 判断有意区分）
	if strings.EqualFold(primaryLanguage(bcp47), "en") {
		return "en-US"
	}
	return fmt.Sprintf("%s, en-US", bcp47)
}

// AndroidScriptedLocale 派生 x-ig-app-locale / x-ig-device-locale header 值
//
// 真机格式(某社交 App Android 429 抓包):
//   - 简体中文:"zh_CN_#Hans"(下划线 + ISO 15924 脚本码,带 # 前缀)
//   - 繁体中文:"zh_TW_#Hant"
//   - 其它(en_IN / pt_BR / es_MX 等):返回原 locale 不加脚本
//
// 接受 "zh_CN" 或 "zh-CN" 两种形式输入,内部统一转下划线
func AndroidScriptedLocale(locale string) string {
	underscored := toUnderscore(locale)
	if script, ok := localeToScript[underscored]; ok {
		return fmt.Sprintf("%s_#%s", underscored, script)
	}
	return underscored
}

// AndroidMappedLocale 派生 x-ig-mapped-locale header 值
//
// 真机格式(某社交 App Android 429 抓包):"zh_CN" / "en_IN" / "pt_BR"(下划线 + 不加脚本)
// 即原 MobileLocaleForCountry 直接输出,只做格式归一化(用户可能传 "zh-CN")
func AndroidMappedLocale(locale string) string {
	return toUnderscore(locale)
}

// AndroidDeviceLanguagesJSON 派生 x-ig-device-languages header 值
//
// 真机格式(某社交 App Android 429 抓包,JSON 字符串):
//
//	{"system_languages":"zh-CN, en-JP","keyboard_language":"en-US"}
//
// 派生策略(实用主义,真机用户输入法多变,我们给一个"合理且安全"的默认):
//   - system_languages = "{primary-bcp47}, en-US"(主语言 + 英文备选,跟 accept-language 同源)
//   - keyboard_language = "en-US"(全球通用键盘,印度/中国/欧洲都典型用英文输入法)
//
// 这跟真机抓包的 "zh-CN, en-JP" 不完全一致(en-JP 是用户偏好),但格式对、字段全,
// 风控只校验"是不是 JSON + 字段对不对",不会要求 system_languages 的具体值匹配。
func AndroidDeviceLanguagesJSON(locale string) string {
	bcp47 := toBCP47(locale)

	// system_languages: 主语言 + en-US 备选(主语言已经是 en 则不重复)
	var systemLangs string
	if strings.EqualFold(primaryLanguage(bcp47), "en") {
		systemLangs = bcp47
	} else {
		systemLangs = fmt.Sprintf("%s, en-US", bcp47)
	}

	// 注意:必须是 JSON 串形式,不带换行,key 用双引号
	// 不用 encoding/json 是因为部分服务端实测对 key 顺序敏感(system_languages 必须在前)
	return fmt.Sprintf(
		`{"system_languages":"%s","keyboard_language":"en-US"}`,
		systemLangs,
	)
}
