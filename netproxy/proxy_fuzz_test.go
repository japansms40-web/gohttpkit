package netproxy

import "testing"

func FuzzParseProxyURL(f *testing.F) {
	f.Add("socks5://127.0.0.1:1080")
	f.Add("socks5://u:p@host.example:1080")
	f.Add("")
	f.Add("http://x")
	f.Add(":")
	f.Fuzz(func(t *testing.T, raw string) {
		d, err := ParseProxyURL(raw)
		if err != nil && d != nil {
			t.Fatalf("err != nil 时 dialer 必须为 nil, raw=%q dialer=%T err=%v", raw, d, err)
		}
	})
}
