package geo

import (
	"sort"
	"strings"
	"testing"
)

func TestMobileLocaleForCountry(t *testing.T) {
	cases := []struct {
		name       string
		country    string
		want       string
		wantErr    bool
		errCountry string
	}{
		{"印尼下划线", "ID", "id_ID", false, ""},
		{"美国下划线", "US", "en_US", false, ""},
		{"日本是 ja_JP 不收成 ja", "JP", "ja_JP", false, ""},
		{"中国下划线", "CN", "zh_CN", false, ""},
		{"巴西下划线", "BR", "pt_BR", false, ""},
		{"小写国家码", "id", "id_ID", false, ""},
		{"首尾空白", " jp ", "ja_JP", false, ""},
		{"未知国家", "XX", "", true, "XX"},
		{"空串", "", "", true, ""},
		{"只空白", "  ", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := MobileLocaleForCountry(c.country)
			t.Logf("country=%q → %q err=%v", c.country, got, err)
			if c.wantErr {
				if got != "" {
					t.Errorf("value = %q, want empty on error", got)
				}
				assertUnknownCountry(t, err, c.errCountry)
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCountryToMobileLocale_值是下划线locale(t *testing.T) {
	keys := make([]string, 0, len(countryToMobileLocale))
	for cc := range countryToMobileLocale {
		keys = append(keys, cc)
	}
	sort.Strings(keys)
	for _, cc := range keys {
		loc := countryToMobileLocale[cc]
		t.Logf("%s → %s", cc, loc)
		if loc == "" {
			t.Errorf("%s 的 Android locale 为空", cc)
			continue
		}
		if !strings.Contains(loc, "_") || strings.Contains(loc, "-") {
			t.Errorf("%s 的 Android locale %q 应是 lang_CC，不能是连字符", cc, loc)
		}
	}
}
