package geo

import "testing"

func TestWebAcceptLanguageForCountry(t *testing.T) {
	cases := []struct {
		country string
		want    string
	}{
		// scratch_39.txt 真实印尼样本，必须严格匹配
		{"ID", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 美国：主语言为 en，不叠 en-US 兜底
		{"US", "en-US,en;q=0.9"},
		// 英国：主语言为 en，输出 en-GB,en;q=0.9
		{"GB", "en-GB,en;q=0.9"},
		// 日本：非 en 主语言，标准三段
		{"JP", "ja-JP,ja;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 巴西：葡语
		{"BR", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 越南
		{"VN", "vi-VN,vi;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 中国
		{"CN", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 印度（用户决策为 en-IN 单一主语言）
		{"IN", "en-IN,en;q=0.9"},
		// 加拿大（en-CA）
		{"CA", "en-CA,en;q=0.9"},
		// 瑞士（de-CH）
		{"CH", "de-CH,de;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 阿联酋（ar-AE）
		{"AE", "ar-AE,ar;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 大小写防御
		{"id", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7"},
		{" jp ", "ja-JP,ja;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 新增加勒比 / 撒哈拉以南非洲 / 葡语非洲国家：守护后续不被无意删表
		{"ZW", "en-ZW,en;q=0.9"},
		{"VC", "en-VC,en;q=0.9"},
		{"BS", "en-BS,en;q=0.9"},
		{"MU", "en-MU,en;q=0.9"},
		{"CW", "nl-CW,nl;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"AO", "pt-AO,pt;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"CV", "pt-CV,pt;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"ML", "fr-ML,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"CD", "fr-CD,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 新增 50 国回归（每种主语言分支挑代表性）
		{"BB", "en-BB,en;q=0.9"},                      // 加勒比英语
		{"FJ", "en-FJ,en;q=0.9"},                      // 太平洋英语
		{"GG", "en-GG,en;q=0.9"},                      // 不列颠群岛
		{"WS", "en-WS,en;q=0.9"},                      // 萨摩亚（GMT+13 极端）
		{"GI", "en-GI,en;q=0.9"},                      // 直布罗陀
		{"ZM", "en-ZM,en;q=0.9"},                      // 赞比亚 CAT
		{"HT", "fr-HT,fr;q=0.9,en-US;q=0.8,en;q=0.7"}, // 法语加勒比
		{"PF", "fr-PF,fr;q=0.9,en-US;q=0.8,en;q=0.7"}, // 法属波利尼西亚
		{"YT", "fr-YT,fr;q=0.9,en-US;q=0.8,en;q=0.7"}, // 印度洋法语
		{"BI", "fr-BI,fr;q=0.9,en-US;q=0.8,en;q=0.7"}, // 法语非洲
		{"GQ", "es-GQ,es;q=0.9,en-US;q=0.8,en;q=0.7"}, // 西语非洲
		{"GW", "pt-GW,pt;q=0.9,en-US;q=0.8,en;q=0.7"}, // 葡语非洲
		{"MR", "ar-MR,ar;q=0.9,en-US;q=0.8,en;q=0.7"}, // 阿语非洲
		{"SO", "so-SO,so;q=0.9,en-US;q=0.8,en;q=0.7"}, // 索马里语
		{"SR", "nl-SR,nl;q=0.9,en-US;q=0.8,en;q=0.7"}, // 荷语苏里南
		// 国家不在表里：返回空，由调用方兜底
		{"XX", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := WebAcceptLanguageForCountry(c.country)
		if got != c.want {
			t.Errorf("WebAcceptLanguageForCountry(%q) = %q, want %q", c.country, got, c.want)
		}
	}
}

func TestMobileLocaleForCountry(t *testing.T) {
	cases := []struct {
		country string
		want    string
	}{
		{"ID", "id_ID"},
		{"US", "en_US"},
		{"GB", "en_GB"},
		{"JP", "ja_JP"},
		{"BR", "pt_BR"},
		{"VN", "vi_VN"},
		{"CN", "zh_CN"},
		{"id", "id_ID"},
		// 新增 24 国回归用例（部分代表）
		{"ZW", "en_ZW"}, {"VC", "en_VC"}, {"BS", "en_BS"}, {"AI", "en_AI"},
		{"GY", "en_GY"}, {"MU", "en_MU"}, {"NA", "en_NA"}, {"SL", "en_SL"},
		{"CW", "nl_CW"},
		{"AO", "pt_AO"}, {"CV", "pt_CV"}, {"MZ", "pt_MZ"}, {"ST", "pt_ST"},
		{"BF", "fr_BF"}, {"BJ", "fr_BJ"}, {"CD", "fr_CD"}, {"ML", "fr_ML"},
		// 新增 50 国回归（下划线格式）
		{"BB", "en_BB"}, {"GD", "en_GD"}, {"KN", "en_KN"}, {"KY", "en_KY"},
		{"LC", "en_LC"}, {"TC", "en_TC"}, {"VG", "en_VG"}, {"VI", "en_VI"},
		{"FJ", "en_FJ"}, {"PG", "en_PG"}, {"WS", "en_WS"},
		{"GG", "en_GG"}, {"IM", "en_IM"}, {"JE", "en_JE"},
		{"BW", "en_BW"}, {"GM", "en_GM"}, {"LR", "en_LR"}, {"SS", "en_SS"}, {"SZ", "en_SZ"}, {"ZM", "en_ZM"},
		{"HT", "fr_HT"}, {"GP", "fr_GP"}, {"MQ", "fr_MQ"}, {"MF", "fr_MF"}, {"GF", "fr_GF"}, {"PM", "fr_PM"},
		{"NC", "fr_NC"}, {"PF", "fr_PF"}, {"WF", "fr_WF"},
		{"KM", "fr_KM"}, {"RE", "fr_RE"}, {"YT", "fr_YT"},
		{"BI", "fr_BI"}, {"CF", "fr_CF"}, {"DJ", "fr_DJ"}, {"GA", "fr_GA"}, {"NE", "fr_NE"}, {"TD", "fr_TD"}, {"TG", "fr_TG"},
		{"GQ", "es_GQ"}, {"GW", "pt_GW"}, {"MR", "ar_MR"}, {"SO", "so_SO"}, {"SR", "nl_SR"},
		{"XX", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := MobileLocaleForCountry(c.country)
		if got != c.want {
			t.Errorf("MobileLocaleForCountry(%q) = %q, want %q", c.country, got, c.want)
		}
	}
}

func TestLocaleToWebAcceptLanguage(t *testing.T) {
	cases := []struct {
		locale string
		want   string
	}{
		// 下划线形式（Android Config.Locale 历史风格）
		{"id_ID", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"zh_CN", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"en_US", "en-US,en;q=0.9"},
		{"en_GB", "en-GB,en;q=0.9"},
		// 连字符形式（Web 风格）
		{"ja-JP", "ja-JP,ja;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 仅主语言（无地区）
		{"fr", "fr,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		// 脏输入
		{"", ""},
		{"!!", ""},
	}
	for _, c := range cases {
		got := localeToWebAcceptLanguage(c.locale)
		if got != c.want {
			t.Errorf("localeToWebAcceptLanguage(%q) = %q, want %q", c.locale, got, c.want)
		}
	}
}

// TestWebAcceptLanguagePriority 验证 Locale > CountryCode > 代理国家 的优先级。
func TestWebAcceptLanguagePriority(t *testing.T) {
	// 1. 仅 CountryCode：用国家映射
	got := WebAcceptLanguage("", "ID", "")
	if got != "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7" {
		t.Errorf("CountryCode-only: got %q", got)
	}

	// 2. Locale 与 CountryCode 同时存在：Locale 优先
	got = WebAcceptLanguage("zh_CN", "ID", "")
	if got != "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7" {
		t.Errorf("Locale > CountryCode: got %q", got)
	}

	// 3. 都为空：返回空（由 NewClient 构造校验拦截，不再兜底 en-US）
	got = WebAcceptLanguage("", "", "")
	if got != "" {
		t.Errorf("Empty config: got %q, want empty", got)
	}

	// 4. 仅代理国家：从 socks5://...country-kr... 解析出口国家派生
	got = WebAcceptLanguage("", "", "socks5://u-country-kr:p@host:1080")
	if got != "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7" {
		t.Errorf("ProxyURL country: got %q", got)
	}
}
