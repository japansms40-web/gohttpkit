package localemobile

import (
	"strconv"
	"strings"
	"testing"
)

// 表头规则：系统语言只有该国一项、键盘语言与之相同，键序与 Android JSONObject 插入顺序一致（紧凑无空格）。
func TestDeviceLanguagesForCountry_紧凑JSON两项都是本国首标签(t *testing.T) {
	for _, cc := range sortedKeys(countryToDeviceLanguages) {
		tag, _, _ := strings.Cut(countryToAcceptLanguage[cc], ", ")
		want := `{"system_languages":` + strconv.Quote(tag) + `,"keyboard_language":` + strconv.Quote(tag) + `}`
		if got := countryToDeviceLanguages[cc]; got != want {
			t.Errorf("%s = %s\nwant %s", cc, got, want)
		}
	}
}
