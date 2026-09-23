package versionreg_test

import (
	"errors"
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

// FuzzRegistryGet 对抗任意版本 ID。
// 不变量：绝不 panic；"" → *EmptyVersionError 且零值；已注册 → 命中同一指针；
// 其余 → *UnknownVersionError 且 Requested 原样、Registered 不含 Requested。
func FuzzRegistryGet(f *testing.F) {
	r := versionreg.New[*versionreg.Config]("fuzz")
	known := versionreg.NewConfig("v1", moreBaseURL)
	r.MustRegister(known)

	for _, s := range []string{"", "v1", "v2", " ", "V1", "v1 ", "v1\n", "版本"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		cfg, err := r.Get(versionreg.ID(s))
		switch s {
		case "":
			if cfg != nil {
				t.Fatal("空 ID 失败应返回零值")
			}
			assertEmptyVersion(t, err, "fuzz", []versionreg.ID{"v1"})
		case "v1":
			if err != nil {
				t.Fatalf("已注册却失败: %v", err)
			}
			if cfg != known {
				t.Fatal("命中应返回表内同一指针")
			}
		default:
			if cfg != nil {
				t.Fatalf("未注册 %q 应返回零值", s)
			}
			var unk *versionreg.UnknownVersionError
			if !errors.As(err, &unk) {
				t.Fatalf("err=%v (%T)，要 *UnknownVersionError", err, err)
			}
			if unk.Requested != versionreg.ID(s) {
				t.Fatalf("Requested=%q 应原样保留 %q", unk.Requested, s)
			}
			for _, have := range unk.Registered {
				if have == unk.Requested {
					t.Fatalf("Registered 含 Requested %q: %v", unk.Requested, unk.Registered)
				}
			}
		}
		if got := r.MustGet("v1"); got != known {
			t.Fatal("fuzz 不得破坏已注册项")
		}
	})
}
