package geo

import (
	"strings"
	"testing"
)

// TestIANATableKeysAreKnownCountries 守护 timezone_iana.go 表头写的维护规约：
// IANA 名称表的每个 key 必须同时存在于 countryToTimezoneOffset。
// 写错国家码（比如把 UK 当成 GB）会让查表静默落空、时区覆盖悄悄失效，
// 这类问题在真机上表现为「页面语言对了但时区不对」，极难排查。
func TestIANATableKeysAreKnownCountries(t *testing.T) {
	for country := range countryToIANATimezone {
		if _, ok := countryToTimezoneOffset[country]; !ok {
			t.Errorf("countryToIANATimezone 含未知国家码 %q（不在 countryToTimezoneOffset 中）", country)
		}
	}
}

// TestIANATimezoneValuesLookSane 挡住手滑：IANA 名称必须是 Area/Location 形式，
// 且不能用 Etc/GMT±N —— 真实用户机器上 Intl 永远不会返回后者，填了就是特征。
func TestIANATimezoneValuesLookSane(t *testing.T) {
	for country, tz := range countryToIANATimezone {
		if !strings.Contains(tz, "/") {
			t.Errorf("%s 的时区 %q 不是 Area/Location 形式", country, tz)
		}
		if strings.HasPrefix(tz, "Etc/") {
			t.Errorf("%s 用了固定偏移时区 %q，真实浏览器不会报这种值", country, tz)
		}
		if strings.TrimSpace(tz) != tz {
			t.Errorf("%s 的时区 %q 含首尾空白", country, tz)
		}
	}
}

func TestIANATimezoneForCountry(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"US", "America/New_York", true},
		{"us", "America/New_York", true},    // 大小写不敏感
		{" br ", "America/Sao_Paulo", true}, // 首尾空白
		{"NL", "Europe/Amsterdam", true},
		{"", "", false},
		{"ZZ", "", false}, // 不存在的国家码
	}
	for _, c := range cases {
		got, ok := IANATimezoneForCountry(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("IANATimezoneForCountry(%q) = (%q, %v), 期望 (%q, %v)",
				c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// TestIANATableCoversCommonExitCountries 锁定注册业务实际用到的出口国不被误删。
// 少一个国家的直接后果是该国出口的会话全部退化成「不覆盖时区」，
// 浏览器报本机时区、IP 在国外 —— 正是协议版 bz_send.go:222 踩过的坑。
func TestIANATableCoversCommonExitCountries(t *testing.T) {
	must := []string{"US", "GB", "BR", "IN", "ID", "PH", "VN", "TH", "MX", "NG", "TR", "RU", "DE", "FR", "NL"}
	for _, c := range must {
		if _, ok := countryToIANATimezone[c]; !ok {
			t.Errorf("常用出口国 %s 从 IANA 时区表里缺失", c)
		}
	}
}
