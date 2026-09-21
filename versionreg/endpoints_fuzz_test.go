package versionreg_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

// FuzzNewHeaderWhitelists 对抗任意 endpoint / 头名 / 头值。
// 不变量：绝不 panic；For 命中值等于入参；改 For 副本或改 src 都不污染集合；
// Has(ep)==true；未写入的另一个键 For 为 nil。
func FuzzNewHeaderWhitelists(f *testing.F) {
	for _, row := range [][3]string{
		{"user.profile", "accept", ""},
		{"", "", ""},
		{" ", "x", "v"},
		{"thread.send", "x-app-id", "123"},
	} {
		f.Add(row[0], row[1], row[2])
	}
	f.Fuzz(func(t *testing.T, ep, key, val string) {
		src := map[versionreg.Endpoint]map[string]string{
			versionreg.Endpoint(ep): {key: val},
		}
		w := versionreg.NewHeaderWhitelists(src)
		if w == nil {
			t.Fatal("构造结果不得为 nil")
		}
		if !w.Has(versionreg.Endpoint(ep)) {
			t.Fatalf("Has(%q) 应为 true", ep)
		}
		got := w.For(versionreg.Endpoint(ep))
		if got == nil {
			t.Fatal("已配置 For 不得为 nil")
		}
		if got[key] != val {
			t.Fatalf("For[%q]=%q, want %q", key, got[key], val)
		}

		got[key] = "mutated-copy"
		got["injected"] = "boom"
		again := w.For(versionreg.Endpoint(ep))
		if again[key] != val {
			t.Fatal("改 For 副本写回了集合")
		}
		if _, leaked := again["injected"]; leaked {
			t.Fatal("For 副本注入泄漏")
		}

		src[versionreg.Endpoint(ep)][key] = "mutated-src"
		src[versionreg.Endpoint(ep)]["src-injected"] = "boom"
		fromSrc := w.For(versionreg.Endpoint(ep))
		if fromSrc[key] != val {
			t.Fatal("改入参 src 污染了集合（未深拷贝）")
		}

		other := versionreg.Endpoint(ep + "\x00other")
		if w.Has(other) || w.For(other) != nil {
			t.Fatal("未写入的键不应命中")
		}
	})
}
