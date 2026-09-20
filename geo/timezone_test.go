package geo

import "testing"

func TestTimezoneOffsetForCountry(t *testing.T) {
	cases := []struct {
		name       string
		country    string
		wantOffset int
		wantErr    bool
		errCountry string
	}{
		{"乌拉圭 UTC-3", "UY", -10800, false, ""},
		{"中国 UTC+8", "CN", 28800, false, ""},
		{"美国东部标准", "US", -18000, false, ""},
		{"日本 UTC+9", "JP", 32400, false, ""},
		{"GMT+0 是合法偏移不是错误", "GB", 0, false, ""},
		{"跨日界正向极端", "WS", 46800, false, ""},
		{"反向极端 GMT-10", "PF", -36000, false, ""},
		{"未知国家", "XX", 0, true, "XX"},
		{"空串", "", 0, true, ""},
		{"只空白", "  ", 0, true, ""},
		{"小写", "uy", -10800, false, ""},
		{"首尾空白", " jp ", 32400, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotOffset, err := TimezoneOffsetForCountry(c.country)
			t.Logf("country=%q → offset=%d err=%v", c.country, gotOffset, err)
			if c.wantErr {
				if gotOffset != 0 {
					t.Errorf("offset = %d, want 0 on error", gotOffset)
				}
				assertUnknownCountry(t, err, c.errCountry)
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if gotOffset != c.wantOffset {
				t.Errorf("got %d, want %d", gotOffset, c.wantOffset)
			}
		})
	}
}

func TestTimezoneTableKeysetMatchesLocaleTable(t *testing.T) {
	assertSameKeys := func(name string, a, b map[string]string) {
		t.Helper()
		for cc := range a {
			if _, ok := b[cc]; !ok {
				t.Errorf("%s 缺少 country %q", name, cc)
			}
		}
	}
	tz := make(map[string]string, len(countryToTimezoneOffset))
	for cc := range countryToTimezoneOffset {
		tz[cc] = ""
	}
	t.Logf("key 数 web=%d mobile=%d tz=%d", len(countryToWebAcceptTag), len(countryToMobileLocale), len(countryToTimezoneOffset))
	assertSameKeys("countryToTimezoneOffset", countryToWebAcceptTag, tz)
	assertSameKeys("countryToWebAcceptTag", tz, countryToWebAcceptTag)
	assertSameKeys("countryToMobileLocale", tz, countryToMobileLocale)
	assertSameKeys("countryToTimezoneOffset(mobile)", countryToMobileLocale, tz)
}
