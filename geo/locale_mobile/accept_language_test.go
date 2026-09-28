package localemobile

import (
	"regexp"
	"strings"
	"testing"
)

// acceptTagRe 是 Accept-Language 首个标签的形态：新语言码 + "-" + 国家，不带 script。
var acceptTagRe = regexp.MustCompile(`^([a-z]{2,3})-([A-Z]{2})$`)

func TestAcceptLanguageForCountry_只有US不追加enUS(t *testing.T) {
	for _, cc := range sortedKeys(countryToAcceptLanguage) {
		v := countryToAcceptLanguage[cc]
		tags := strings.Split(v, ", ")
		if cc == "US" {
			if v != "en-US" {
				t.Errorf("US = %q，应只有 en-US", v)
			}
			continue
		}
		if len(tags) != 2 || tags[1] != "en-US" {
			t.Errorf("%s = %q，非 US 应恰好是「本国标签, en-US」", cc, v)
		}
	}
}

func TestAcceptLanguageForCountry_首标签用新语言码且国家段等于键(t *testing.T) {
	legacy := map[string]bool{"in": true, "iw": true, "ji": true}
	for _, cc := range sortedKeys(countryToAcceptLanguage) {
		first, _, _ := strings.Cut(countryToAcceptLanguage[cc], ", ")
		m := acceptTagRe.FindStringSubmatch(first)
		if m == nil {
			t.Errorf("%s: 首标签 %q 不是 lang-CC 形态（不应带 script 或下划线）", cc, first)
			continue
		}
		if legacy[m[1]] {
			t.Errorf("%s: 首标签 %q 用了 Android 旧码，应换回 id/he/yi", cc, first)
		}
		if m[2] != cc {
			t.Errorf("%s: 首标签 %q 的国家段应为 %s", cc, first, cc)
		}
	}
	for cc, want := range map[string]string{"ID": "id-ID, en-US", "IL": "he-IL, en-US"} {
		got, err := AcceptLanguageForCountry(cc)
		t.Logf("%s → %q err=%v", cc, got, err)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v；want %q", cc, got, err, want)
		}
	}
}
