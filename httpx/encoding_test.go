package httpx

import (
	"strings"
	"testing"
)

func TestParseContentEncoding_归一化为枚举(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want ContentEncoding
		ok   bool
	}{
		{"zstd", "zstd", EncodingZstd, true},
		{"大写 GZIP", "GZIP", EncodingGzip, true},
		{"混用 Deflate", "Deflate", EncodingDeflate, true},
		{"br", "br", EncodingBr, true},
		{"空串当未压缩", "", EncodingIdentity, false},
		{"未知 identity", "identity", EncodingIdentity, false},
		{"复合编码不拆", "gzip, deflate", EncodingIdentity, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseContentEncoding(tc.raw)
			t.Logf("raw=%q → encoding=%q ok=%v", tc.raw, got, ok)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ParseContentEncoding(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestContentEncoding_String是字面值(t *testing.T) {
	t.Logf("identity=%q zstd=%q gzip=%q deflate=%q br=%q",
		EncodingIdentity, EncodingZstd, EncodingGzip, EncodingDeflate, EncodingBr)
	if EncodingIdentity.String() != "" ||
		EncodingZstd.String() != "zstd" ||
		EncodingGzip.String() != "gzip" ||
		EncodingDeflate.String() != "deflate" ||
		EncodingBr.String() != "br" {
		t.Fatal("枚举字面值必须与 content-encoding 头一致")
	}
}

// FuzzParseContentEncoding 对抗任意 content-encoding 原文。
// 不变量（不会误报）：绝不 panic；ok=true ⟹ 枚举 ∈ 登记集且 == ToLower(输入)；ok=false ⟹ EncodingIdentity。
func FuzzParseContentEncoding(f *testing.F) {
	for _, s := range []string{"gzip", "GZIP", "", "br", "deflate", "zstd", "identity", "gzip, br", " gzip", "GzIp", "x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		enc, ok := ParseContentEncoding(s)
		if ok {
			switch enc {
			case EncodingZstd, EncodingGzip, EncodingDeflate, EncodingBr:
			default:
				t.Fatalf("ok=true 但 enc=%q 不在登记集", enc)
			}
			if string(enc) != strings.ToLower(s) {
				t.Fatalf("ok=true 但 enc=%q != ToLower(%q)", enc, s)
			}
		} else if enc != EncodingIdentity {
			t.Fatalf("ok=false 但 enc=%q 非 EncodingIdentity", enc)
		}
	})
}
