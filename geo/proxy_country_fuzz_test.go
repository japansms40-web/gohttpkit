package geo

import "testing"

func FuzzParseCountryFromProxyURL(f *testing.F) {
	f.Add("socks5://acc-country-us:pwd@gate:2031")
	f.Add("socks5://user:base_country-BR_session-1@host:1080")
	f.Add("")
	f.Add("not a url")
	f.Fuzz(func(t *testing.T, raw string) {
		cc := ParseCountryFromProxyURL(raw)
		if cc != "" && len(cc) != 2 {
			t.Fatalf("country = %q, want empty or 2 letters", cc)
		}
	})
}
