package versionreg

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
)

func newReg(t *testing.T) *Registry[*Config] {
	t.Helper()
	return New[*Config]("test")
}

func TestRegistry_注册与取用(t *testing.T) {
	r := newReg(t)
	r.MustRegister(NewConfig("v1", "https://a.example",
		WithUserAgent("ua/1"),
		WithParams(map[string]string{"app_id": "123"}),
		WithHeaderWhitelists(map[Endpoint]map[string]string{
			"user.profile": {"accept": "", "x-app-id": "123"},
		}),
		WithDocIDs(map[Endpoint]string{"user.profile": "doc-9"}),
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
	r.MustRegister(NewConfig("v1", "https://a.example"))

	_, err := r.Get("")
	t.Logf("Get(\"\") → %v", err)
	assertEmptyVersion(t, err, "test", []ID{"v1"})

	_, err = r.Get("v2")
	t.Logf("Get(\"v2\") → %v", err)
	assertUnknownVersion(t, err, "test", "v2", []ID{"v1"})
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
		r.MustRegister(NewConfig("v1", "https://a.example"))
		r.MustRegister(NewConfig("v1", "https://b.example"))
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
		newReg(t).MustRegister(NewConfig("v1", ""))
	})
}

func TestRegistry_List字典序(t *testing.T) {
	r := newReg(t)
	for _, id := range []ID{"v3", "v1", "v2"} {
		r.MustRegister(NewConfig(id, "https://a.example"))
	}
	got := r.List()
	t.Logf("List = %v", got)
	if len(got) != 3 || got[0] != "v1" || got[2] != "v3" {
		t.Fatalf("List = %v, want 字典序", got)
	}
}

func TestID与Endpoint_String(t *testing.T) {
	t.Logf("ID.String = %q Endpoint.String = %q", ID("v1").String(), Endpoint("user.profile").String())
	if ID("v1").String() != "v1" {
		t.Fatal("ID.String 应返回原串")
	}
	if Endpoint("user.profile").String() != "user.profile" {
		t.Fatal("Endpoint.String 应返回原串")
	}
}

func TestRegistry_HasLenMustGet(t *testing.T) {
	r := newReg(t)
	if r.Len() != 0 {
		t.Fatalf("空注册表 Len = %d", r.Len())
	}
	r.MustRegister(NewConfig("v1", "https://a.example"))

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
	r := New[emptyIDConfig]("test")
	r.MustRegister(emptyIDConfig{})
}

// emptyIDConfig 是 Validate 通过但 VersionID 为空的病态配置。
type emptyIDConfig struct{}

func (emptyIDConfig) VersionID() ID   { return "" }
func (emptyIDConfig) Validate() error { return nil }

func TestRegistry_并发注册与Get快照一致(t *testing.T) {
	r := New[*Config]("test")
	const n = 40
	var wg sync.WaitGroup
	errCh := make(chan string, 64)

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := ID(fmt.Sprintf("v%03d", i))
			r.MustRegister(NewConfig(id, "https://a.example"))
		}(i)
	}
	for range 8 {
		wg.Go(func() {
			for j := range n {
				id := ID(fmt.Sprintf("v%03d", j))
				_, err := r.Get(id)
				if err == nil {
					_ = r.Has(id)
					_ = r.List()
					continue
				}
				var unk *UnknownVersionError
				if !errors.As(err, &unk) {
					errCh <- fmt.Sprintf("Get(%q) = %v (%T)，只允许成功或 *UnknownVersionError", id, err, err)
					return
				}
				if slices.Contains(unk.Registered, unk.Requested) {
					errCh <- fmt.Sprintf("未知错误 Registered 含 Requested %q: %v（#4 逻辑 TOCTOU）", unk.Requested, unk.Registered)
					return
				}
				_ = r.Has(id)
				_ = r.List()
			}
		})
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
	for i := range n {
		id := ID(fmt.Sprintf("v%03d", i))
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

// —— 角度补充：按 docs/TESTING.md §2 补 Registry / ID 的角度测试。
// 上方用例已锁 happy + 基本错误；这里补边界、零值、契约、
// 状态迁移、输入多样性。out-of-scope：零值 Registry（文档写明不可用）、
// 资源生命周期（无 Close）、IO。

const moreBaseURL = "https://a.example"

func mustPanic(t *testing.T, fn func()) any {
	t.Helper()
	var recovered any
	func() {
		defer func() {
			recovered = recover()
			t.Logf("recover=%v (%T)", recovered, recovered)
			if recovered == nil {
				t.Fatal("应 panic")
			}
		}()
		fn()
	}()
	return recovered
}

func mustPanicError(t *testing.T, fn func()) error {
	t.Helper()
	r := mustPanic(t, fn)
	err, ok := r.(error)
	if !ok {
		t.Fatalf("panic 值应是 error，得到 %T", r)
	}
	return err
}

func TestID_String边界与不做trim(t *testing.T) {
	cases := []struct {
		in   ID
		want string
	}{
		{"", ""},
		{" ", " "},
		{"  v1  ", "  v1  "},
		{"v1", "v1"},
		{"V1", "V1"},
		{"版本-1", "版本-1"},
		{ID("v1\n"), "v1\n"},
	}
	for _, c := range cases {
		got := c.in.String()
		t.Logf("ID(%q).String() = %q", string(c.in), got)
		if got != c.want {
			t.Errorf("ID(%q).String() = %q, want %q（契约：原样返回、不 trim）", string(c.in), got, c.want)
		}
	}
}

func TestID_String实现fmtStringer(t *testing.T) {
	id := ID("web-2026-06-22")
	got := fmt.Sprint(id)
	t.Logf("fmt.Sprint(ID(%q)) = %q", string(id), got)
	if got != "web-2026-06-22" {
		t.Fatalf("fmt.Stringer 应打出底层串，得到 %q", got)
	}
}

func TestNew_空name仍可用且出现在错误里(t *testing.T) {
	r := New[*Config]("")
	t.Logf("New(\"\") Len=%d List=%v", r.Len(), r.List())
	if r.Len() != 0 {
		t.Fatalf("空表 Len = %d", r.Len())
	}
	_, err := r.Get("")
	assertEmptyVersion(t, err, "", nil)
	_, err = r.Get("v9")
	assertUnknownVersion(t, err, "", "v9", nil)
}

func TestNew_name不是键只进错误(t *testing.T) {
	r := New[*Config]("android")
	r.MustRegister(NewConfig("v1", moreBaseURL))
	if r.Has("android") {
		t.Fatal("New 的 name 不得当成版本键")
	}
	_, err := r.Get("android")
	t.Logf("Get(name) → %v", err)
	assertUnknownVersion(t, err, "android", "android", []ID{"v1"})
}

func TestMustRegister_失败不写入(t *testing.T) {
	r := newReg(t)

	err := mustPanicError(t, func() {
		r.MustRegister(NewConfig("v1", ""))
	})
	assertMissingField(t, err, "BaseURL")
	if r.Len() != 0 || r.Has("v1") {
		t.Fatalf("Validate 失败后表应仍空，Len=%d Has=%v", r.Len(), r.Has("v1"))
	}

	r.MustRegister(NewConfig("v1", moreBaseURL))
	dup := mustPanicError(t, func() {
		r.MustRegister(NewConfig("v1", "https://b.example"))
	})
	if !kiterrors.IsKind(dup, kiterrors.NewKind("versionreg.register.duplicate")) ||
		!strings.Contains(dup.Error(), "registry=test") || !strings.Contains(dup.Error(), "version=v1") {
		t.Fatalf("重复注册 panic 应含分类、表名和版本: %v", dup)
	}
	if r.Len() != 1 {
		t.Fatalf("重复注册 panic 后 Len 应仍为 1，得到 %d", r.Len())
	}
	if got := r.MustGet("v1"); got.BaseURL != moreBaseURL {
		t.Fatalf("重复注册不得覆盖已有项: %+v", got)
	}

	empty := New[emptyIDConfig]("test")
	emptyErr := mustPanicError(t, func() {
		empty.MustRegister(emptyIDConfig{})
	})
	if !kiterrors.IsKind(emptyErr, kiterrors.NewKind("versionreg.register.empty_id")) ||
		!strings.Contains(emptyErr.Error(), "registry=test") {
		t.Fatalf("空 VersionID panic 应含分类和表名: %v", emptyErr)
	}
	if empty.Len() != 0 {
		t.Fatalf("空 VersionID panic 后不应写入，Len=%d", empty.Len())
	}
}

var errBoomValidate = errors.New("boom")

func TestMustRegister_Validate任意错误仍包装可Is(t *testing.T) {
	err := mustPanicError(t, func() {
		r := New[boomCfg]("test")
		r.MustRegister(boomCfg{})
	})
	t.Logf("自定义 Validate 错误包装后 %v", err)
	if !errors.Is(err, errBoomValidate) {
		t.Fatalf("Validate 错误应以 %%w 包装，errors.Is 应命中原因，得到 %v", err)
	}
	if !kiterrors.IsKind(err, kiterrors.NewKind("versionreg.register.invalid")) {
		t.Fatalf("Validate 错误应有结构化分类，得到 %v", err)
	}
}

type boomCfg struct{}

func (boomCfg) VersionID() ID   { return "v1" }
func (boomCfg) Validate() error { return errBoomValidate }

func TestMustRegister_指针身份共享实例(t *testing.T) {
	r := newReg(t)
	cfg := NewConfig("v1", moreBaseURL, WithUserAgent("ua/1"))
	r.MustRegister(cfg)

	got, err := r.Get("v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Get 指针 %p 注册指针 %p", got, cfg)
	if got != cfg {
		t.Fatal("T 为指针时 Get 应返回表内同一实例")
	}
	cfg.UserAgent = "changed"
	if r.MustGet("v1").UserAgent != "changed" {
		t.Fatal("共享实例：改注册指针应反映到下次 Get（契约：注册后勿改）")
	}
}

func TestMustRegister_值类型可注册且Get是副本(t *testing.T) {
	r := New[valCfg]("test")
	r.MustRegister(valCfg{id: "v1", mark: "orig"})
	got, err := r.Get("v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("值类型 Get = %+v", got)
	if got.mark != "orig" {
		t.Fatalf("Get = %+v", got)
	}
	got.mark = "mutated"
	again, err := r.Get("v1")
	if err != nil {
		t.Fatal(err)
	}
	if again.mark != "orig" {
		t.Fatal("值类型 Get 应是副本，改返回值不得写回表")
	}
}

type valCfg struct {
	id   ID
	mark string
}

func (v valCfg) VersionID() ID   { return v.id }
func (v valCfg) Validate() error { return nil }

func TestMustRegister_空白ID可注册(t *testing.T) {
	r := newReg(t)
	r.MustRegister(NewConfig("  ", moreBaseURL))
	t.Logf("Has(\"  \")=%v Has(\"\")=%v Has(\" \")=%v", r.Has("  "), r.Has(""), r.Has(" "))
	if !r.Has("  ") {
		t.Fatal("空白（非空）ID 应按原样注册")
	}
	if r.Has("") || r.Has(" ") {
		t.Fatal("Has 不做 trim，\"\" / \" \" 不得命中 \"  \"")
	}
	got, err := r.Get("  ")
	if err != nil || got.ID != "  " {
		t.Fatalf("Get(\"  \") = %+v err=%v", got, err)
	}
}

func TestGet_空表(t *testing.T) {
	r := newReg(t)
	cfg, err := r.Get("")
	t.Logf("空表 Get(\"\") cfg=%v err=%v", cfg, err)
	if cfg != nil {
		t.Fatal("失败应返回零值 T（指针类型为 nil）")
	}
	assertEmptyVersion(t, err, "test", nil)

	cfg, err = r.Get("v1")
	t.Logf("空表 Get(\"v1\") cfg=%v err=%v", cfg, err)
	if cfg != nil {
		t.Fatal("失败应返回零值 T")
	}
	assertUnknownVersion(t, err, "test", "v1", nil)
}

func TestGet_空白与大小写不trim(t *testing.T) {
	r := newReg(t)
	r.MustRegister(NewConfig("v1", moreBaseURL))

	cases := []ID{" ", "v1 ", " v1", "V1", "v1\t"}
	for _, id := range cases {
		cfg, err := r.Get(id)
		t.Logf("Get(%q) cfg=%v err=%v", string(id), cfg, err)
		if cfg != nil {
			t.Fatalf("Get(%q) 不应命中 v1（不做 trim / 区分大小写）", string(id))
		}
		assertUnknownVersion(t, err, "test", id, []ID{"v1"})
		if r.Has(id) {
			t.Fatalf("Has(%q) 应为 false", string(id))
		}
	}
}

func TestGet_Unknown_Registered是新切片(t *testing.T) {
	r := newReg(t)
	r.MustRegister(NewConfig("v2", moreBaseURL))
	r.MustRegister(NewConfig("v1", moreBaseURL))

	_, err := r.Get("v9")
	var unk *UnknownVersionError
	if !errors.As(err, &unk) {
		t.Fatalf("err=%v", err)
	}
	unk.Registered[0] = "mutated"
	list := r.List()
	t.Logf("改错误 Registered 后 List=%v", list)
	if list[0] != "v1" || list[1] != "v2" {
		t.Fatal("UnknownVersionError.Registered 必须是新切片")
	}
}

func TestMustGet_空版本panic是EmptyVersionError(t *testing.T) {
	r := newReg(t)
	r.MustRegister(NewConfig("v1", moreBaseURL))
	err := mustPanicError(t, func() { r.MustGet("") })
	assertEmptyVersion(t, err, "test", []ID{"v1"})
}

func TestMustGet_命中返回同一指针(t *testing.T) {
	r := newReg(t)
	cfg := NewConfig("v1", moreBaseURL)
	r.MustRegister(cfg)
	got := r.MustGet("v1")
	t.Logf("MustGet 指针 %p 注册指针 %p", got, cfg)
	if got != cfg {
		t.Fatal("MustGet 命中应与 Get 一样返回表内同一指针")
	}
}

func TestList_空表与单元素(t *testing.T) {
	r := newReg(t)
	empty := r.List()
	t.Logf("空表 List=%v nil=%v", empty, empty == nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("空表 List 应为长度 0 的切片（非 nil），得到 %v", empty)
	}

	r.MustRegister(NewConfig("v1", moreBaseURL))
	one := r.List()
	t.Logf("单元素 List=%v", one)
	if len(one) != 1 || one[0] != "v1" {
		t.Fatalf("List = %v", one)
	}
}

func TestList_返回切片隔离(t *testing.T) {
	r := newReg(t)
	r.MustRegister(NewConfig("v1", moreBaseURL))
	got := r.List()
	got[0] = "mutated"
	again := r.List()
	t.Logf("改第一次 List 后再次 List=%v", again)
	if again[0] != "v1" {
		t.Fatal("List 必须返回新切片，就地改写不得污染表")
	}
}

func TestList_字典序不是semver(t *testing.T) {
	r := newReg(t)
	for _, id := range []ID{"v10", "v2", "v1"} {
		r.MustRegister(NewConfig(id, moreBaseURL))
	}
	got := r.List()
	t.Logf("List(%v) = %v", []string{"v10", "v2", "v1"}, got)
	want := []ID{"v1", "v10", "v2"}
	if !idSliceEqual(got, want) {
		t.Fatalf("List = %v, want 字典序 %v（不是 semver：v10 应排在 v2 前）", got, want)
	}
}

func TestHas_空与空白与大小写(t *testing.T) {
	r := newReg(t)
	if r.Has("") || r.Has("v1") {
		t.Fatal("空表 Has 应为 false")
	}
	r.MustRegister(NewConfig("v1", moreBaseURL))
	cases := []struct {
		id   ID
		want bool
	}{
		{"v1", true},
		{"", false},
		{" ", false},
		{"v1 ", false},
		{"V1", false},
		{"v9", false},
	}
	for _, c := range cases {
		got := r.Has(c.id)
		t.Logf("Has(%q) = %v want %v", string(c.id), got, c.want)
		if got != c.want {
			t.Errorf("Has(%q) = %v, want %v", string(c.id), got, c.want)
		}
	}
}

func TestLen_随成功注册递增(t *testing.T) {
	r := newReg(t)
	if r.Len() != 0 {
		t.Fatalf("空表 Len=%d", r.Len())
	}
	r.MustRegister(NewConfig("v1", moreBaseURL))
	if r.Len() != 1 {
		t.Fatalf("注册 1 个后 Len=%d", r.Len())
	}
	r.MustRegister(NewConfig("v2", moreBaseURL))
	t.Logf("注册 2 个后 Len=%d", r.Len())
	if r.Len() != 2 {
		t.Fatalf("Len=%d, want 2", r.Len())
	}
}

// TestKindRegister_导出变量锁定名称 导出 Kind 是契约：名称与 v0.11.0 起的字面量一致，
// 且 MustRegister 三种 panic 可直接用导出变量判定，调用方不必再手写魔法串。
func TestKindRegister_导出变量锁定名称(t *testing.T) {
	cases := []struct {
		name string
		kind kiterrors.Kind
		want string
		fire func()
	}{
		{"校验失败", KindRegisterInvalid, "versionreg.register.invalid", func() {
			New[boomCfg]("test").MustRegister(boomCfg{})
		}},
		{"空版本", KindRegisterEmptyID, "versionreg.register.empty_id", func() {
			New[emptyIDConfig]("test").MustRegister(emptyIDConfig{})
		}},
		{"重复注册", KindRegisterDuplicate, "versionreg.register.duplicate", func() {
			r := New[*Config]("test")
			r.MustRegister(NewConfig("v1", moreBaseURL))
			r.MustRegister(NewConfig("v1", moreBaseURL))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.kind != kiterrors.NewKind(c.want) || c.kind.Name() != c.want {
				t.Fatalf("Kind 名称漂移：%v, want %q", c.kind, c.want)
			}
			err := mustPanicError(t, c.fire)
			if !kiterrors.IsKind(err, c.kind) {
				t.Fatalf("panic 应带 %q 分类，得到 %v", c.want, err)
			}
		})
	}
}

// FuzzRegistryGet 对抗任意版本 ID。
// 不变量：绝不 panic；"" → *EmptyVersionError 且零值；已注册 → 命中同一指针；
// 其余 → *UnknownVersionError 且 Requested 原样、Registered 不含 Requested。
func FuzzRegistryGet(f *testing.F) {
	r := New[*Config]("fuzz")
	known := NewConfig("v1", moreBaseURL)
	r.MustRegister(known)

	for _, s := range []string{"", "v1", "v2", " ", "V1", "v1 ", "v1\n", "版本"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		cfg, err := r.Get(ID(s))
		switch s {
		case "":
			if cfg != nil {
				t.Fatal("空 ID 失败应返回零值")
			}
			assertEmptyVersion(t, err, "fuzz", []ID{"v1"})
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
			var unk *UnknownVersionError
			if !errors.As(err, &unk) {
				t.Fatalf("err=%v (%T)，要 *UnknownVersionError", err, err)
			}
			if unk.Requested != ID(s) {
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

// —— 内部实现：未导出 copyIDsLocked / sortIDs 的角度测试。
// 这两层是 List / 错误 Registered 快照的单一实现，只靠导出 API 测不到
// nil 切片、就地排序、空表容量这些分支。

func TestCopyIDsLocked_空表是长度0的新切片(t *testing.T) {
	r := New[*Config]("t")
	r.mu.RLock()
	ids := r.copyIDsLocked()
	r.mu.RUnlock()
	t.Logf("空表 copyIDsLocked = %v nil=%v cap=%d", ids, ids == nil, cap(ids))
	if ids == nil || len(ids) != 0 {
		t.Fatalf("空表应返回长度 0 的切片（非 nil），得到 %v", ids)
	}
	injected := append(ids, "injected")
	if len(injected) != 1 || injected[0] != "injected" {
		t.Fatalf("append 结果应为单元素，得到 %v", injected)
	}
	r.mu.RLock()
	again := r.copyIDsLocked()
	r.mu.RUnlock()
	if len(again) != 0 {
		t.Fatal("append 返回切片不得写回 items")
	}
}

func TestCopyIDsLocked_单元素与多元素成员完整(t *testing.T) {
	r := New[*Config]("t")
	r.MustRegister(NewConfig("only", "https://a.example"))
	r.mu.RLock()
	one := r.copyIDsLocked()
	r.mu.RUnlock()
	t.Logf("单元素 = %v", one)
	if len(one) != 1 || one[0] != "only" {
		t.Fatalf("单元素 copy = %v", one)
	}

	r.MustRegister(NewConfig("a", "https://a.example"))
	r.MustRegister(NewConfig("z", "https://a.example"))
	r.mu.RLock()
	many := r.copyIDsLocked()
	r.mu.RUnlock()
	t.Logf("多元素（未排序）= %v", many)
	if len(many) != 3 {
		t.Fatalf("len=%d, want 3", len(many))
	}
	seen := map[ID]int{}
	for _, id := range many {
		seen[id]++
	}
	for _, want := range []ID{"only", "a", "z"} {
		if seen[want] != 1 {
			t.Fatalf("缺/重 %q: %v", want, many)
		}
	}
}

func TestCopyIDsLocked_两次调用互不共享底层数组(t *testing.T) {
	r := New[*Config]("t")
	r.MustRegister(NewConfig("v1", "https://a.example"))
	r.MustRegister(NewConfig("v2", "https://a.example"))
	r.mu.RLock()
	a := r.copyIDsLocked()
	b := r.copyIDsLocked()
	r.mu.RUnlock()
	a[0] = "mutated"
	t.Logf("改 a 后 b=%v", b)
	for _, id := range b {
		if id == "mutated" {
			t.Fatal("两次 copyIDsLocked 不得共享底层数组")
		}
	}
}

func TestSortIDs_nil与空切片不panic(t *testing.T) {
	sortIDs(nil)
	empty := []ID{}
	sortIDs(empty)
	t.Logf("nil / 空切片 sort 后 empty=%v", empty)
	if len(empty) != 0 {
		t.Fatalf("空切片 sort 后 len=%d", len(empty))
	}
}

func TestSortIDs_单元素保持(t *testing.T) {
	ids := []ID{"only"}
	sortIDs(ids)
	t.Logf("单元素 = %v", ids)
	if len(ids) != 1 || ids[0] != "only" {
		t.Fatalf("单元素被改成 %v", ids)
	}
}

func TestSortIDs_已排序保持_逆序排开(t *testing.T) {
	sorted := []ID{"a", "b", "c"}
	sortIDs(sorted)
	t.Logf("已排序 = %v", sorted)
	if sorted[0] != "a" || sorted[2] != "c" {
		t.Fatalf("已排序被打乱: %v", sorted)
	}

	rev := []ID{"c", "b", "a"}
	sortIDs(rev)
	t.Logf("逆序 → %v", rev)
	if rev[0] != "a" || rev[1] != "b" || rev[2] != "c" {
		t.Fatalf("逆序未排开: %v", rev)
	}
}

func TestSortIDs_重复键保持相邻且就地改(t *testing.T) {
	ids := []ID{"b", "a", "a", "c"}
	orig := ids
	sortIDs(ids)
	t.Logf("含重复 → %v", ids)
	want := []ID{"a", "a", "b", "c"}
	if len(ids) != 4 || ids[0] != "a" || ids[1] != "a" || ids[2] != "b" || ids[3] != "c" {
		t.Fatalf("got %v, want %v", ids, want)
	}
	if &ids[0] != &orig[0] {
		t.Fatal("sortIDs 应就地改同一切片")
	}
}

func TestSortIDs_字典序不是semver(t *testing.T) {
	ids := []ID{"v10", "v2", "v1"}
	sortIDs(ids)
	t.Logf("v10/v2/v1 → %v", ids)
	if ids[0] != "v1" || ids[1] != "v10" || ids[2] != "v2" {
		t.Fatalf("应是字典序 [v1 v10 v2]，得到 %v", ids)
	}
}
