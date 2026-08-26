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

// ─────────────────────────── 访问器与边界补全 ───────────────────────────

func TestID与Endpoint_String(t *testing.T) {
	if versionreg.ID("v1").String() != "v1" {
		t.Fatal("ID.String 应返回原串")
	}
	if versionreg.Endpoint("user.profile").String() != "user.profile" {
		t.Fatal("Endpoint.String 应返回原串")
	}
}

func TestRegistry_HasLenMustGet(t *testing.T) {
	r := newReg(t)
	if r.Len() != 0 {
		t.Fatalf("空注册表 Len = %d", r.Len())
	}
	r.MustRegister(versionreg.NewConfig("v1", "https://a.example"))

	if !r.Has("v1") {
		t.Fatal("Has 应命中已注册版本")
	}
	if r.Has("v9") {
		t.Fatal("Has 不该命中未注册版本")
	}
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}
	if got := r.MustGet("v1"); got.BaseURL != "https://a.example" {
		t.Fatalf("MustGet = %+v", got)
	}
}

func TestRegistry_MustGet未注册时panic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	newReg(t).MustGet("nope")
}

func TestRegistry_空版本标识panic(t *testing.T) {
	// Validate 过得去、但 VersionID 为空的配置也必须被挡住，
	// 否则注册表里会出现一个永远取不出来的幽灵条目。
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	r := versionreg.New[emptyIDConfig]("test")
	r.MustRegister(emptyIDConfig{})
}

// emptyIDConfig 是 Validate 通过但 VersionID 为空的病态配置。
type emptyIDConfig struct{}

func (emptyIDConfig) VersionID() versionreg.ID { return "" }
func (emptyIDConfig) Validate() error          { return nil }

func TestConfig_未配置项返回零值(t *testing.T) {
	cfg := versionreg.NewConfig("v1", "https://a.example")
	if cfg.DocID("nope") != "" {
		t.Fatal("未配置 docIDs 时应返回空串")
	}
	if cfg.Param("nope") != "" {
		t.Fatal("未配置 Params 时应返回空串")
	}
	if cfg.HeaderWhitelist("nope") != nil {
		t.Fatal("未配置白名单时应返回 nil(= 发全量头)")
	}
}

func TestConfig_Validate缺ID(t *testing.T) {
	cfg := versionreg.NewConfig("", "https://a.example")
	if err := cfg.Validate(); err == nil {
		t.Fatal("缺 ID 应报错")
	}
}

func TestConfig_Option可叠加(t *testing.T) {
	cfg := versionreg.NewConfig("v1", "https://a.example",
		versionreg.WithParams(map[string]string{"a": "1"}),
		versionreg.WithParams(map[string]string{"b": "2"}), // 第二次应合并而非覆盖
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"e1": "d1"}),
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"e2": "d2"}),
	)
	if cfg.Param("a") != "1" || cfg.Param("b") != "2" {
		t.Fatalf("WithParams 应合并, got %v", cfg.Params)
	}
	if cfg.DocID("e1") != "d1" || cfg.DocID("e2") != "d2" {
		t.Fatal("WithDocIDs 应合并")
	}
}

func TestHeaderWhitelists_HasEndpoints与深拷贝(t *testing.T) {
	src := map[versionreg.Endpoint]map[string]string{
		"b.ep": {"accept": ""},
		"a.ep": {"accept": "", "x-k": "v"},
	}
	w := versionreg.NewHeaderWhitelists(src)

	if !w.Has("a.ep") || w.Has("zzz") {
		t.Fatal("Has 判定不对")
	}
	eps := w.Endpoints()
	if len(eps) != 2 || eps[0] != "a.ep" || eps[1] != "b.ep" {
		t.Fatalf("Endpoints = %v, want 字典序", eps)
	}
	if w.For("zzz") != nil {
		t.Fatal("未配置的 endpoint 应返回 nil")
	}

	// 构造时深拷贝：改入参不该影响已构造的集合
	src["a.ep"]["injected"] = "boom"
	if _, leaked := w.For("a.ep")["injected"]; leaked {
		t.Fatal("NewHeaderWhitelists 未做深拷贝")
	}
}
