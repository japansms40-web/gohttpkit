package localemobile

import (
	"regexp"
	"testing"
)

// androidLocaleRe 是 Locale.toString() 形态：语言_国家，有 script 时接 "_#Script"。
var androidLocaleRe = regexp.MustCompile(`^([a-z]{2,3})_([A-Z]{2})(?:_#([A-Z][a-z]{3}))?$`)

func TestDeviceLocaleForCountry_是toString形态且国家段等于键(t *testing.T) {
	for _, cc := range sortedKeys(countryToDeviceLocale) {
		v := countryToDeviceLocale[cc]
		m := androidLocaleRe.FindStringSubmatch(v)
		if m == nil {
			t.Errorf("%s: %q 不是 lang_CC[_#Script] 形态", cc, v)
			continue
		}
		if m[2] != cc {
			t.Errorf("%s: %q 的国家段应为 %s", cc, v, cc)
		}
	}
}

func TestDeviceLocaleForCountry_保留Android旧语言码(t *testing.T) {
	for cc, want := range map[string]string{"ID": "in_ID", "IL": "iw_IL"} {
		got, err := DeviceLocaleForCountry(cc)
		t.Logf("%s → %q err=%v", cc, got, err)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v；want %q（toString 用旧码）", cc, got, err, want)
		}
	}
	for _, cc := range sortedKeys(countryToDeviceLocale) {
		m := androidLocaleRe.FindStringSubmatch(countryToDeviceLocale[cc])
		if m != nil && (m[1] == "id" || m[1] == "he" || m[1] == "yi") {
			t.Errorf("%s: %q 用了新码，toString 应是 in/iw/ji", cc, countryToDeviceLocale[cc])
		}
	}
}
