package httpx_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

func TestParseContentEncoding_归一化为枚举(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want httpx.ContentEncoding
		ok   bool
	}{
		{"zstd", "zstd", httpx.EncodingZstd, true},
		{"大写 GZIP", "GZIP", httpx.EncodingGzip, true},
		{"混用 Deflate", "Deflate", httpx.EncodingDeflate, true},
		{"br", "br", httpx.EncodingBr, true},
		{"空串当未压缩", "", httpx.EncodingIdentity, false},
		{"未知 identity", "identity", httpx.EncodingIdentity, false},
		{"复合编码不拆", "gzip, deflate", httpx.EncodingIdentity, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := httpx.ParseContentEncoding(tc.raw)
			t.Logf("raw=%q → encoding=%q ok=%v", tc.raw, got, ok)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ParseContentEncoding(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestContentEncoding_String是字面值(t *testing.T) {
	t.Logf("identity=%q zstd=%q gzip=%q deflate=%q br=%q",
		httpx.EncodingIdentity, httpx.EncodingZstd, httpx.EncodingGzip, httpx.EncodingDeflate, httpx.EncodingBr)
	if httpx.EncodingIdentity.String() != "" ||
		httpx.EncodingZstd.String() != "zstd" ||
		httpx.EncodingGzip.String() != "gzip" ||
		httpx.EncodingDeflate.String() != "deflate" ||
		httpx.EncodingBr.String() != "br" {
		t.Fatal("枚举字面值必须与 content-encoding 头一致")
	}
}
