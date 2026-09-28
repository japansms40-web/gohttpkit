package localemobile

import (
	"strings"
	"testing"
)

// script 只出现在 supported_locales 只给带 script 形式的国家。
func TestInvariant_script范围(t *testing.T) {
	want := map[string]string{
		"CN": "Hans", "TW": "Hant", "HK": "Hant", "MO": "Hant",
		"RS": "Cyrl", "ME": "Latn", "BA": "Latn", "AZ": "Latn", "UZ": "Latn",
	}
	for _, cc := range sortedKeys(countryToAppLocale) {
		_, script, _ := strings.Cut(countryToAppLocale[cc], "_#")
		if script != want[cc] {
			t.Errorf("%s: script = %q, want %q", cc, script, want[cc])
		}
	}
}
