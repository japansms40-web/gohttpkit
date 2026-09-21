package httpx

import (
	"strings"
	"testing"
)

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
