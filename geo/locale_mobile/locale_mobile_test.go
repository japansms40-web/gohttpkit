package localemobile

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/geo"
)

// headerFunc 是五个查表函数的共同签名，便于同一组用例跑遍每个 header。
type headerFunc struct {
	name  string
	fn    func(string) (string, error)
	table map[string]string
}

var headerFuncs = []headerFunc{
	{"AcceptLanguage", AcceptLanguageForCountry, countryToAcceptLanguage},
	{"AppLocale", AppLocaleForCountry, countryToAppLocale},
	{"DeviceLocale", DeviceLocaleForCountry, countryToDeviceLocale},
	{"MappedLocale", MappedLocaleForCountry, countryToMappedLocale},
	{"DeviceLanguages", DeviceLanguagesForCountry, countryToDeviceLanguages},
}

func assertUnknownCountry(t *testing.T, err error, country string) {
	t.Helper()
	var target *geo.UnknownCountryError
	if !errors.As(err, &target) {
		t.Fatalf("err = %v, want *geo.UnknownCountryError", err)
	}
	if target.Country != country {
		t.Errorf("Country = %q, want %q", target.Country, country)
	}
}

func sortedKeys(table map[string]string) []string {
	keys := make([]string, 0, len(table))
	for cc := range table {
		keys = append(keys, cc)
	}
	sort.Strings(keys)
	return keys
}

// 五个 header 的代表性国家；CN 行对照抓包 00299。
func TestForCountry_代表国家(t *testing.T) {
	cases := []struct {
		country                                              string
		acceptLanguage, appLocale, deviceLocale, mapped, dls string
	}{
		{"CN", "zh-CN, en-US", "zh_CN_#Hans", "zh_CN_#Hans", "zh_CN", `{"system_languages":"zh-CN","keyboard_language":"zh-CN"}`},
		{"US", "en-US", "en_US", "en_US", "en_US", `{"system_languages":"en-US","keyboard_language":"en-US"}`},
		{"GB", "en-GB, en-US", "en_GB", "en_GB", "en_GB", `{"system_languages":"en-GB","keyboard_language":"en-GB"}`},
		{"AU", "en-AU, en-US", "en_AU", "en_AU", "en_US", `{"system_languages":"en-AU","keyboard_language":"en-AU"}`},
		{"JP", "ja-JP, en-US", "ja_JP", "ja_JP", "ja_JP", `{"system_languages":"ja-JP","keyboard_language":"ja-JP"}`},
		{"ID", "id-ID, en-US", "in_ID", "in_ID", "id_ID", `{"system_languages":"id-ID","keyboard_language":"id-ID"}`},
		{"IL", "he-IL, en-US", "iw_IL", "iw_IL", "he_IL", `{"system_languages":"he-IL","keyboard_language":"he-IL"}`},
		{"TW", "zh-TW, en-US", "zh_TW_#Hant", "zh_TW_#Hant", "zh_TW", `{"system_languages":"zh-TW","keyboard_language":"zh-TW"}`},
		{"HK", "zh-HK, en-US", "zh_HK_#Hant", "zh_HK_#Hant", "zh_HK", `{"system_languages":"zh-HK","keyboard_language":"zh-HK"}`},
		// zh-Hant-MO 三段未命中，退到 zh-Hant → zh_TW。
		{"MO", "zh-MO, en-US", "zh_MO_#Hant", "zh_MO_#Hant", "zh_TW", `{"system_languages":"zh-MO","keyboard_language":"zh-MO"}`},
		{"RS", "sr-RS, en-US", "sr_RS_#Cyrl", "sr_RS_#Cyrl", "sr_RS", `{"system_languages":"sr-RS","keyboard_language":"sr-RS"}`},
		{"AZ", "az-AZ, en-US", "az_AZ_#Latn", "az_AZ_#Latn", "az_AZ", `{"system_languages":"az-AZ","keyboard_language":"az-AZ"}`},
		{"MX", "es-MX, en-US", "es_MX", "es_MX", "es_LA", `{"system_languages":"es-MX","keyboard_language":"es-MX"}`},
		{"ES", "es-ES, en-US", "es_ES", "es_ES", "es_ES", `{"system_languages":"es-ES","keyboard_language":"es-ES"}`},
		{"PT", "pt-PT, en-US", "pt_PT", "pt_PT", "pt_PT", `{"system_languages":"pt-PT","keyboard_language":"pt-PT"}`},
		{"BR", "pt-BR, en-US", "pt_BR", "pt_BR", "pt_BR", `{"system_languages":"pt-BR","keyboard_language":"pt-BR"}`},
		{"CA", "en-CA, en-US", "en_CA", "en_CA", "en_US", `{"system_languages":"en-CA","keyboard_language":"en-CA"}`},
		{"BT", "dz-BT, en-US", "dz_BT", "dz_BT", "en_US", `{"system_languages":"dz-BT","keyboard_language":"dz-BT"}`},
	}
	for _, c := range cases {
		want := []string{c.acceptLanguage, c.appLocale, c.deviceLocale, c.mapped, c.dls}
		for i, h := range headerFuncs {
			t.Run(c.country+"/"+h.name, func(t *testing.T) {
				got, err := h.fn(c.country)
				t.Logf("%s(%q) → %q err=%v", h.name, c.country, got, err)
				if err != nil {
					t.Fatalf("unexpected err %v", err)
				}
				if got != want[i] {
					t.Errorf("got %q, want %q", got, want[i])
				}
			})
		}
	}
}

func TestForCountry_输入归一化(t *testing.T) {
	for _, h := range headerFuncs {
		want := h.table["CN"]
		for _, in := range []string{"cn", " CN ", "\tcN\n"} {
			got, err := h.fn(in)
			t.Logf("%s(%q) → %q err=%v", h.name, in, got, err)
			if err != nil || got != want {
				t.Errorf("%s(%q) = %q, %v; want %q", h.name, in, got, err, want)
			}
		}
	}
}

func TestForCountry_错误(t *testing.T) {
	cases := []struct{ in, errCountry string }{
		{"", ""}, {"  ", ""}, {"XX", "XX"}, {" xx ", "XX"}, {"CHN", "CHN"}, {"zh-CN", "ZH-CN"},
	}
	for _, h := range headerFuncs {
		for _, c := range cases {
			got, err := h.fn(c.in)
			t.Logf("%s(%q) → %q err=%v", h.name, c.in, got, err)
			if got != "" {
				t.Errorf("%s(%q) value = %q, want empty on error", h.name, c.in, got)
			}
			assertUnknownCountry(t, err, c.errCountry)
		}
	}
}

// keyset：五张表相同，且与 geo 的国家表（经导出函数扫全部两字母码）一致。
func TestKeyset_五表一致且与geo对齐(t *testing.T) {
	want := sortedKeys(countryToAcceptLanguage)
	for _, h := range headerFuncs {
		got := sortedKeys(h.table)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s keyset 与 AcceptLanguage 不同：%d vs %d", h.name, len(got), len(want))
		}
	}
	var fromGeo []string
	for a := 'A'; a <= 'Z'; a++ {
		for b := 'A'; b <= 'Z'; b++ {
			cc := string([]rune{a, b})
			if _, err := geo.MobileLocaleForCountry(cc); err == nil {
				fromGeo = append(fromGeo, cc)
			}
		}
	}
	t.Logf("localemobile %d 国，geo %d 国", len(want), len(fromGeo))
	if strings.Join(fromGeo, ",") != strings.Join(want, ",") {
		t.Errorf("与 geo 国家表不一致\n geo: %v\nhere: %v", fromGeo, want)
	}
}

// 全表不变量：五个值之间满足 APK 的派生关系，与 geo.MobileLocaleForCountry 的语言、国家一致。
func TestInvariant_全表派生关系(t *testing.T) {
	newCode := map[string]string{"in": "id", "iw": "he", "ji": "yi"}
	// geo 表里 MO 借用 zh_HK；本包按 Android 可选列表是 zh-Hant-MO。
	geoOverride := map[string]string{"MO": "zh_MO"}
	for _, cc := range sortedKeys(countryToAcceptLanguage) {
		al := countryToAcceptLanguage[cc]
		app := countryToAppLocale[cc]
		dev := countryToDeviceLocale[cc]
		mapped := countryToMappedLocale[cc]
		dls := countryToDeviceLanguages[cc]

		if app != dev {
			t.Errorf("%s: app %q != device %q", cc, app, dev)
		}
		base, _, _ := strings.Cut(dev, "_#")
		lang, country, ok := strings.Cut(base, "_")
		if !ok || country != cc {
			t.Errorf("%s: device %q 国家段应为 %s", cc, dev, cc)
		}
		if n, ok := newCode[lang]; ok {
			lang = n
		}
		want, err := geo.MobileLocaleForCountry(cc)
		if err != nil {
			t.Fatalf("%s: geo err %v", cc, err)
		}
		if v, ok := geoOverride[cc]; ok {
			want = v
		}
		if got := lang + "_" + country; got != want {
			t.Errorf("%s: device %q 还原为 %q，geo 为 %q", cc, dev, got, want)
		}

		tag := lang + "-" + country
		wantAL := tag + ", en-US"
		if cc == "US" {
			wantAL = "en-US"
		}
		if al != wantAL {
			t.Errorf("%s: AcceptLanguage = %q, want %q", cc, al, wantAL)
		}

		if mapped == "" || strings.Contains(mapped, "-") || strings.Contains(mapped, "#") {
			t.Errorf("%s: MappedLocale %q 应是非空下划线形式", cc, mapped)
		}

		if !strings.HasPrefix(dls, `{"system_languages":`) {
			t.Errorf("%s: DeviceLanguages 键顺序应先 system_languages：%q", cc, dls)
		}
		var parsed map[string]string
		if err := json.Unmarshal([]byte(dls), &parsed); err != nil {
			t.Fatalf("%s: DeviceLanguages 不是合法 JSON %q: %v", cc, dls, err)
		}
		if len(parsed) != 2 || parsed["system_languages"] != tag || parsed["keyboard_language"] != tag {
			t.Errorf("%s: DeviceLanguages = %v, want 两项都是 %q", cc, parsed, tag)
		}
	}
}

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
