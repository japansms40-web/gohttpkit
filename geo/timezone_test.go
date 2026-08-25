package geo

import "testing"

// TestTimezoneOffsetForCountry 关键样本覆盖：用户场景、典型大时区、GMT+0、未命中、大小写。
func TestTimezoneOffsetForCountry(t *testing.T) {
	cases := []struct {
		country    string
		wantOffset int
		wantOK     bool
	}{
		// 用户场景关键样本
		{"UY", -10800, true},
		// 典型大时区
		{"CN", 28800, true},
		{"US", -18000, true},
		{"AU", 36000, true},
		{"JP", 32400, true},
		{"IN", 19800, true},
		{"BR", -10800, true},
		// GMT+0：验证 0 是合法返回（ok=true）
		{"GB", 0, true},
		{"IS", 0, true},
		{"PT", 0, true},
		{"IE", 0, true},
		// 新增 24 国时区回归（按地理分组各取一个）
		{"ZW", 7200, true},   // 撒哈拉非洲 CAT
		{"VC", -14400, true}, // 加勒比 AST
		{"CW", -14400, true}, // 加勒比荷兰王国
		{"AO", 3600, true},   // 葡语非洲 WAT
		{"CV", -3600, true},  // 佛得角 CVT
		{"MV", 18000, true},  // 马尔代夫 MVT
		{"ML", 0, true},      // 法语非洲 GMT
		{"GN", 0, true},      // 法语非洲 GMT
		// 新增 50 国时区回归（覆盖跨日界极端、GMT+0 新成员、加勒比 AST、SAST/CAT、太平洋 +12）
		{"WS", 46800, true},  // 萨摩亚（GMT+13 跨日界正向极端）
		{"PF", -36000, true}, // 法属波利尼西亚（GMT-10 反向极端）
		{"FJ", 43200, true},  // 斐济 FJT +12
		{"WF", 43200, true},  // 瓦利斯富图纳 WFT +12
		{"NC", 39600, true},  // 新喀里多尼亚 NCT +11
		{"PW", 32400, true},  // 帕劳 PWT +9
		{"GU", 36000, true},  // 关岛 ChST +10
		{"PG", 36000, true},  // 巴新 PGT +10
		// GMT+0 新成员（守护 0 仍是合法 offset）
		{"GM", 0, true}, // 冈比亚
		{"LR", 0, true}, // 利比里亚
		{"TG", 0, true}, // 多哥
		{"GG", 0, true}, // 根西岛
		{"IM", 0, true}, // 马恩岛
		{"JE", 0, true}, // 泽西岛
		{"GW", 0, true}, // 几内亚比绍
		{"MR", 0, true}, // 毛里塔尼亚
		// 加勒比 AST
		{"BB", -14400, true}, // 巴巴多斯
		{"LC", -14400, true}, // 圣卢西亚
		{"GP", -14400, true}, // 瓜德罗普
		{"MQ", -14400, true}, // 马提尼克
		{"VI", -14400, true}, // 美属维京
		// 美东 EST
		{"HT", -18000, true}, // 海地
		{"KY", -18000, true}, // 开曼
		{"TC", -18000, true}, // 特克斯凯科斯
		// CAT / SAST / WAT / EAT 非洲
		{"ZM", 7200, true},  // 赞比亚 CAT
		{"BI", 7200, true},  // 布隆迪 CAT
		{"SS", 7200, true},  // 南苏丹（2021 改 CAT）
		{"SZ", 7200, true},  // 斯威士兰 SAST
		{"LS", 7200, true},  // 莱索托 SAST
		{"BW", 7200, true},  // 博茨瓦纳 CAT
		{"CF", 3600, true},  // 中非 WAT
		{"GA", 3600, true},  // 加蓬 WAT
		{"GQ", 3600, true},  // 赤道几内亚 WAT
		{"NE", 3600, true},  // 尼日尔 WAT
		{"TD", 3600, true},  // 乍得 WAT
		{"DJ", 10800, true}, // 吉布提 EAT
		{"KM", 10800, true}, // 科摩罗 EAT
		{"YT", 10800, true}, // 马约特 EAT
		{"SO", 10800, true}, // 索马里 EAT
		// 其他特殊偏移
		{"GF", -10800, true}, // 法属圭亚那 GMT-3
		{"PM", -10800, true}, // 圣皮埃尔密克隆 PMST
		{"SR", -10800, true}, // 苏里南 SRT
		{"BZ", -21600, true}, // 伯利兹 CST
		{"GI", 3600, true},   // 直布罗陀 CET
		{"RE", 14400, true},  // 留尼汪 RET
		// 未命中
		{"XX", 0, false},
		{"", 0, false},
		{"  ", 0, false},
		// 大小写 / trim 防御
		{"uy", -10800, true},
		{" jp ", 32400, true},
		{"Cn", 28800, true},
	}

	for _, c := range cases {
		gotOffset, gotOK := TimezoneOffsetForCountry(c.country)
		if gotOffset != c.wantOffset || gotOK != c.wantOK {
			t.Errorf("TimezoneOffsetForCountry(%q) = (%d, %v), want (%d, %v)",
				c.country, gotOffset, gotOK, c.wantOffset, c.wantOK)
		}
	}
}

// TestTimezoneTableKeysetMatchesLocaleTable 守护 timezone 表与 countryToMobileLocale
// keyset 完全一致——任一边新增 country 都必须同步加，否则 CI 红。
func TestTimezoneTableKeysetMatchesLocaleTable(t *testing.T) {
	missingInTimezone := []string{}
	for cc := range countryToMobileLocale {
		if _, ok := countryToTimezoneOffset[cc]; !ok {
			missingInTimezone = append(missingInTimezone, cc)
		}
	}
	if len(missingInTimezone) > 0 {
		t.Errorf("countryToTimezoneOffset 缺少这些 country（已在 countryToMobileLocale 中）：%v", missingInTimezone)
	}

	missingInLocale := []string{}
	for cc := range countryToTimezoneOffset {
		if _, ok := countryToMobileLocale[cc]; !ok {
			missingInLocale = append(missingInLocale, cc)
		}
	}
	if len(missingInLocale) > 0 {
		t.Errorf("countryToMobileLocale 缺少这些 country（已在 countryToTimezoneOffset 中）：%v", missingInLocale)
	}
}
