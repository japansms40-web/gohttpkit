package geo

import (
	"sort"
	"strings"
	"testing"
)

// TestIANATableKeysAreKnownCountries 守护 timezone_iana.go 表头写的维护规约：
// IANA 名称表的每个 key 必须同时存在于 countryToTimezoneOffset。
// 写错国家码（比如把 UK 当成 GB）会让查表静默落空、时区覆盖悄悄失效，
// 这类问题在真机上表现为「页面语言对了但时区不对」，极难排查。
func TestIANATableKeysAreKnownCountries(t *testing.T) {
	t.Logf("IANA 表 %d 国，须都在 offset 表（%d 国）里", len(countryToIANATimezone), len(countryToTimezoneOffset))
	keys := make([]string, 0, len(countryToIANATimezone))
	for country := range countryToIANATimezone {
		keys = append(keys, country)
	}
	sort.Strings(keys)
	for _, country := range keys {
		t.Logf("IANA key %s 在 offset 表", country)
		if _, ok := countryToTimezoneOffset[country]; !ok {
			t.Errorf("countryToIANATimezone 含未知国家码 %q（不在 countryToTimezoneOffset 中）", country)
		}
	}
}

// TestIANATimezoneValuesLookSane 挡住手滑：IANA 名称必须是 Area/Location 形式，
// 且不能用 Etc/GMT±N —— 真实用户机器上 Intl 永远不会返回后者，填了就是特征。
func TestIANATimezoneValuesLookSane(t *testing.T) {
	keys := make([]string, 0, len(countryToIANATimezone))
	for country := range countryToIANATimezone {
		keys = append(keys, country)
	}
	sort.Strings(keys)
	for _, country := range keys {
		tz := countryToIANATimezone[country]
		t.Logf("%s → %s", country, tz)
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
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"美国纽约", "US", "America/New_York", true},
		{"小写", "us", "America/New_York", true},
		{"首尾空白", " br ", "America/Sao_Paulo", true},
		{"荷兰", "NL", "Europe/Amsterdam", true},
		{"空串未收录", "", "", false},
		{"只空白未收录", "   ", "", false},
		{"未知国家未收录", "ZZ", "", false},
		{"offset 表有但 IANA 子集没有", "AD", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := IANATimezoneForCountry(c.in)
			t.Logf("in=%q → %q ok=%v", c.in, got, ok)
			if got != c.want || ok != c.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestIANATableCoversCommonExitCountries 锁定注册业务实际用到的出口国不被误删。
// 少一个国家的直接后果是该国出口的会话全部退化成「不覆盖时区」，
// 浏览器报本机时区、IP 在国外 —— 正是协议版 bz_send.go:222 踩过的坑。
func TestIANATableCoversCommonExitCountries(t *testing.T) {
	must := []string{"US", "GB", "BR", "IN", "ID", "PH", "VN", "TH", "LA", "MX", "NG", "TR", "RU", "DE", "FR", "NL"}
	for _, c := range must {
		tz, ok := countryToIANATimezone[c]
		t.Logf("常用出口国 %s → %s ok=%v", c, tz, ok)
		if !ok {
			t.Errorf("常用出口国 %s 从 IANA 时区表里缺失", c)
		}
	}
}
