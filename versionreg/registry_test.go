package versionreg_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func newReg(t *testing.T) *versionreg.Registry[*versionreg.Config] {
	t.Helper()
	return versionreg.New[*versionreg.Config]("test")
}

func TestRegistry_注册与取用(t *testing.T) {
	r := newReg(t)
	r.MustRegister(versionreg.NewConfig("v1", "https://a.example",
		versionreg.WithUserAgent("ua/1"),
		versionreg.WithParams(map[string]string{"app_id": "123"}),
		versionreg.WithHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
			"user.profile": {"accept": "", "x-app-id": "123"},
		}),
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"user.profile": "doc-9"}),
	))

	cfg, err := r.Get("v1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserAgent != "ua/1" || cfg.Param("app_id") != "123" || cfg.DocID("user.profile") != "doc-9" {
		t.Fatalf("cfg = %+v", cfg)
	}
	wl := cfg.HeaderWhitelist("user.profile")
	if wl["x-app-id"] != "123" {
		t.Fatalf("whitelist = %v", wl)
	}
	// 拿到的必须是副本：就地改不该影响下一次取用
	wl["injected"] = "boom"
	if _, leaked := cfg.HeaderWhitelist("user.profile")["injected"]; leaked {
		t.Fatal("白名单返回了内部 map，被就地改写污染了")
	}
}

func TestRegistry_未注册与空版本报错不回退(t *testing.T) {
	r := newReg(t)
	r.MustRegister(versionreg.NewConfig("v1", "https://a.example"))

	if _, err := r.Get(""); err == nil {
		t.Fatal("空版本应报错，不该回退到唯一已注册版本")
	}
	if _, err := r.Get("v2"); err == nil {
		t.Fatal("未注册版本应报错")
	}
}

func TestRegistry_重复注册与非法配置panic(t *testing.T) {
	t.Run("重复", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("want panic")
			}
		}()
		r := newReg(t)
		r.MustRegister(versionreg.NewConfig("v1", "https://a.example"))
		r.MustRegister(versionreg.NewConfig("v1", "https://b.example"))
	})
	t.Run("缺必填字段", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("want panic")
			}
		}()
		newReg(t).MustRegister(versionreg.NewConfig("v1", "")) // BaseURL 空
	})
}

func TestRegistry_List字典序(t *testing.T) {
	r := newReg(t)
	for _, id := range []versionreg.ID{"v3", "v1", "v2"} {
		r.MustRegister(versionreg.NewConfig(id, "https://a.example"))
	}
	got := r.List()
	if len(got) != 3 || got[0] != "v1" || got[2] != "v3" {
		t.Fatalf("List = %v, want 字典序", got)
	}
}

func TestHeaderWhitelists_nil安全(t *testing.T) {
	var w *versionreg.HeaderWhitelists
	if w.For("x") != nil || w.Has("x") || w.Endpoints() != nil {
		t.Fatal("nil 白名单集合应安全返回零值")
	}
}
