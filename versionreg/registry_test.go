package versionreg_test

import (
	"errors"
	"fmt"
	"sync"
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
	t.Logf("Get(v1) → ua=%q app_id=%q doc=%q", cfg.UserAgent, cfg.Param("app_id"), cfg.DocID("user.profile"))
	if cfg.UserAgent != "ua/1" || cfg.Param("app_id") != "123" || cfg.DocID("user.profile") != "doc-9" {
		t.Fatalf("cfg = %+v", cfg)
	}
	wl := cfg.HeaderWhitelist("user.profile")
	if wl["x-app-id"] != "123" {
		t.Fatalf("whitelist = %v", wl)
	}
	wl["injected"] = "boom"
	if _, leaked := cfg.HeaderWhitelist("user.profile")["injected"]; leaked {
		t.Fatal("白名单返回了内部 map，被就地改写污染了")
	}
}

func TestRegistry_未注册与空版本报错不回退(t *testing.T) {
	r := newReg(t)
	r.MustRegister(versionreg.NewConfig("v1", "https://a.example"))

	_, err := r.Get("")
	t.Logf("Get(\"\") → %v", err)
	assertEmptyVersion(t, err, "test", []versionreg.ID{"v1"})

	_, err = r.Get("v2")
	t.Logf("Get(\"v2\") → %v", err)
	assertUnknownVersion(t, err, "test", "v2", []versionreg.ID{"v1"})
}

func TestRegistry_重复注册与非法配置panic(t *testing.T) {
	t.Run("重复", func(t *testing.T) {
		defer func() {
			r := recover()
			t.Logf("重复注册 recover=%v", r)
			if r == nil {
				t.Fatal("重复注册应 panic")
			}
		}()
		r := newReg(t)
		r.MustRegister(versionreg.NewConfig("v1", "https://a.example"))
		r.MustRegister(versionreg.NewConfig("v1", "https://b.example"))
	})
	t.Run("缺必填字段", func(t *testing.T) {
		defer func() {
			r := recover()
			t.Logf("缺 BaseURL recover=%v", r)
			if r == nil {
				t.Fatal("缺 BaseURL 应 panic")
			}
			err, ok := r.(error)
			if !ok {
				t.Fatalf("panic 值应是 error，得到 %T", r)
			}
			assertMissingField(t, err, "BaseURL")
		}()
		newReg(t).MustRegister(versionreg.NewConfig("v1", ""))
	})
}

func TestRegistry_List字典序(t *testing.T) {
	r := newReg(t)
	for _, id := range []versionreg.ID{"v3", "v1", "v2"} {
		r.MustRegister(versionreg.NewConfig(id, "https://a.example"))
	}
	got := r.List()
	t.Logf("List = %v", got)
	if len(got) != 3 || got[0] != "v1" || got[2] != "v3" {
		t.Fatalf("List = %v, want 字典序", got)
	}
}

func TestID与Endpoint_String(t *testing.T) {
	t.Logf("ID.String = %q Endpoint.String = %q", versionreg.ID("v1").String(), versionreg.Endpoint("user.profile").String())
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

	t.Logf("Has(v1)=%v Has(v9)=%v Len=%d", r.Has("v1"), r.Has("v9"), r.Len())
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
		r := recover()
		t.Logf("recover=%v (%T)", r, r)
		if r == nil {
			t.Fatal("未注册 MustGet 应 panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic 值应是 error，得到 %T", r)
		}
		assertUnknownVersion(t, err, "test", "nope", nil)
	}()
	newReg(t).MustGet("nope")
}

func TestRegistry_空版本标识panic(t *testing.T) {
	defer func() {
		r := recover()
		t.Logf("空 VersionID recover=%v", r)
		if r == nil {
			t.Fatal("空版本标识应 panic")
		}
	}()
	r := versionreg.New[emptyIDConfig]("test")
	r.MustRegister(emptyIDConfig{})
}

// emptyIDConfig 是 Validate 通过但 VersionID 为空的病态配置。
type emptyIDConfig struct{}

func (emptyIDConfig) VersionID() versionreg.ID { return "" }
func (emptyIDConfig) Validate() error          { return nil }

func TestRegistry_并发注册与Get快照一致(t *testing.T) {
	r := versionreg.New[*versionreg.Config]("test")
	const n = 40
	var wg sync.WaitGroup
	errCh := make(chan string, 64)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := versionreg.ID(fmt.Sprintf("v%03d", i))
			r.MustRegister(versionreg.NewConfig(id, "https://a.example"))
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				id := versionreg.ID(fmt.Sprintf("v%03d", j))
				_, err := r.Get(id)
				if err == nil {
					_ = r.Has(id)
					_ = r.List()
					continue
				}
				var unk *versionreg.UnknownVersionError
				if !errors.As(err, &unk) {
					errCh <- fmt.Sprintf("Get(%q) = %v (%T)，只允许成功或 *UnknownVersionError", id, err, err)
					return
				}
				for _, have := range unk.Registered {
					if have == unk.Requested {
						errCh <- fmt.Sprintf("未知错误 Registered 含 Requested %q: %v（#4 逻辑 TOCTOU）", unk.Requested, unk.Registered)
						return
					}
				}
				_ = r.Has(id)
				_ = r.List()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
	if r.Len() != n {
		t.Fatalf("Len = %d, want %d", r.Len(), n)
	}
	list := r.List()
	t.Logf("最终 Len=%d List=%v", r.Len(), list)
	for i := 0; i < n; i++ {
		id := versionreg.ID(fmt.Sprintf("v%03d", i))
		if !r.Has(id) {
			t.Fatalf("最终应有 %s", id)
		}
	}
	for i := 1; i < len(list); i++ {
		if list[i-1] >= list[i] {
			t.Fatalf("List 未排序: %v", list)
		}
	}
}
