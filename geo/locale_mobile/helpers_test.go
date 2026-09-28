package localemobile

import (
	"errors"
	"sort"
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
