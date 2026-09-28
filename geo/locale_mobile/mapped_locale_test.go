package localemobile

import (
	"regexp"
	"strings"
	"testing"
)

// mappedLocaleRe 是 Meta 内部 locale 形态：语言_地区，无 script、无连字符。
var mappedLocaleRe = regexp.MustCompile(`^[a-z]{2,3}_[A-Z]{2}$`)

// primaryLang 取该国 Accept-Language 首标签的语言段（新码）。
func primaryLang(cc string) string {
	lang, _, _ := strings.Cut(countryToAcceptLanguage[cc], "-")
	return lang
}

func TestMappedLocaleForCountry_形态为语言_地区(t *testing.T) {
	for _, cc := range sortedKeys(countryToMappedLocale) {
		if v := countryToMappedLocale[cc]; !mappedLocaleRe.MatchString(v) {
			t.Errorf("%s: %q 不是 lang_RR 形态", cc, v)
		}
	}
}

// 表头规则：en 只有 GB 保留 en_GB，其余英语区都是 en_US；es 默认 es_LA（ES 为 es_ES）；pt 默认 pt_BR（PT 为 pt_PT）。
func TestMappedLocaleForCountry_英西葡按默认区域归并(t *testing.T) {
	rules := map[string]struct{ keep, fallback string }{
		"en": {"GB", "en_US"},
		"es": {"ES", "es_LA"},
		"pt": {"PT", "pt_BR"},
	}
	for _, cc := range sortedKeys(countryToMappedLocale) {
		lang := primaryLang(cc)
		r, ok := rules[lang]
		if !ok {
			continue
		}
		want := r.fallback
		if cc == r.keep {
			want = lang + "_" + cc
		}
		if got := countryToMappedLocale[cc]; got != want {
			t.Errorf("%s（%s）: mapped = %q, want %q", cc, lang, got, want)
		}
	}
}

func TestMappedLocaleForCountry_用新语言码(t *testing.T) {
	for cc, want := range map[string]string{"ID": "id_ID", "IL": "he_IL"} {
		got, err := MappedLocaleForCountry(cc)
		t.Logf("%s → %q err=%v", cc, got, err)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v；want %q（mapped 走 toLanguageTag，是新码）", cc, got, err, want)
		}
	}
}
