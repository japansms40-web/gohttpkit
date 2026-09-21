package geo

import (
	"strings"
	"testing"
)

// FuzzBuildChromeAcceptLanguage 对抗任意标签列表（用 '\n' 切成 tags）。
// 不变量：绝不 panic；不改入参切片（doc 承诺）；同输入确定性输出；
// q 值良构（无 q=0.0 / q=1. / 负 q；跳过 tag 本身就含 ";q=" 的对抗串以免误报）。
func FuzzBuildChromeAcceptLanguage(f *testing.F) {
	for _, s := range []string{"zh-CN\nen\nja", "", "en-US\nen-GB", "x-private\ni-klingon", "  \n  ", "a\na\na", "-\n--\n-x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		tags := strings.Split(s, "\n")
		orig := append([]string(nil), tags...)

		got := BuildChromeAcceptLanguage(tags) // 绝不 panic

		for i := range tags {
			if tags[i] != orig[i] {
				t.Fatalf("改了入参切片[%d]：%q → %q", i, orig[i], tags[i])
			}
		}
		if got2 := BuildChromeAcceptLanguage(orig); got2 != got {
			t.Fatalf("非确定性输出：%q vs %q", got, got2)
		}

		skipQ := false
		for _, tg := range tags {
			if strings.Contains(tg, ";q=") {
				skipQ = true
				break
			}
		}
		if !skipQ {
			for _, bad := range []string{"q=0.0", "q=1.", "q=-", "q=0.\n"} {
				if strings.Contains(got, bad) {
					t.Fatalf("q 值畸形（含 %q）：%q", bad, got)
				}
			}
		}
	})
}
