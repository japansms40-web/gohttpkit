package netproxy

import "testing"

func TestParseScheme_归一化为枚举(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Scheme
		ok   bool
	}{
		{"socks5", "socks5", SchemeSOCKS5, true},
		{"大写 SOCKS5", "SOCKS5", SchemeSOCKS5, true},
		{"http", "http", SchemeHTTP, true},
		{"混用 Http", "Http", SchemeHTTP, true},
		{"https", "https", SchemeHTTPS, true},
		{"ftp 未登记", "ftp", "", false},
		{"空串", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseScheme(tc.raw)
			t.Logf("raw=%q → scheme=%q ok=%v", tc.raw, got, ok)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parseScheme(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestScheme_零值String是空串(t *testing.T) {
	var z Scheme
	t.Logf("零值 String=%q", z.String())
	if z.String() != "" {
		t.Fatalf("零值 Scheme.String() = %q", z.String())
	}
}

func TestScheme_String是字面值(t *testing.T) {
	t.Logf("SOCKS5=%q HTTP=%q HTTPS=%q", SchemeSOCKS5, SchemeHTTP, SchemeHTTPS)
	if SchemeSOCKS5.String() != "socks5" || SchemeHTTP.String() != "http" || SchemeHTTPS.String() != "https" {
		t.Fatal("枚举字面值必须与 URL scheme 一致")
	}
}
