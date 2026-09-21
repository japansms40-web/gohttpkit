package versionreg_test

// registry_more_test.go —— 按 docs/TESTING.md §2 补 Registry / ID 的角度测试。
// 现有 registry_test.go 已锁 happy + 基本错误；这里补边界、零值、契约、
// 状态迁移、输入多样性。out-of-scope：零值 Registry（文档写明不可用）、
// 资源生命周期（无 Close）、IO。

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

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

func mustPanicString(t *testing.T, fn func()) string {
	t.Helper()
	r := mustPanic(t, fn)
	s, ok := r.(string)
	if !ok {
		t.Fatalf("panic 值应是 string，得到 %T（Validate 失败才是 error）", r)
	}
	return s
}

func TestID_String边界与不做trim(t *testing.T) {
	cases := []struct {
		in   versionreg.ID
		want string
	}{
		{"", ""},
		{" ", " "},
		{"  v1  ", "  v1  "},
		{"v1", "v1"},
		{"V1", "V1"},
		{"版本-1", "版本-1"},
		{versionreg.ID("v1\n"), "v1\n"},
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
	id := versionreg.ID("web-2026-06-22")
	got := fmt.Sprint(id)
	t.Logf("fmt.Sprint(ID(%q)) = %q", string(id), got)
	if got != "web-2026-06-22" {
		t.Fatalf("fmt.Stringer 应打出底层串，得到 %q", got)
	}
}

func TestNew_空name仍可用且出现在错误里(t *testing.T) {
	r := versionreg.New[*versionreg.Config]("")
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
	r := versionreg.New[*versionreg.Config]("android")
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
	if r.Has("android") {
		t.Fatal("New 的 name 不得当成版本键")
	}
	_, err := r.Get("android")
	t.Logf("Get(name) → %v", err)
	assertUnknownVersion(t, err, "android", "android", []versionreg.ID{"v1"})
}

func TestMustRegister_失败不写入(t *testing.T) {
	r := newReg(t)

	err := mustPanicError(t, func() {
		r.MustRegister(versionreg.NewConfig("v1", ""))
	})
	assertMissingField(t, err, "BaseURL")
	if r.Len() != 0 || r.Has("v1") {
		t.Fatalf("Validate 失败后表应仍空，Len=%d Has=%v", r.Len(), r.Has("v1"))
	}

	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
	dup := mustPanicString(t, func() {
		r.MustRegister(versionreg.NewConfig("v1", "https://b.example"))
	})
	t.Logf("重复注册 panic=%q Len=%d", dup, r.Len())
	if !strings.Contains(dup, "test") || !strings.Contains(dup, "v1") {
		t.Fatalf("重复注册 panic 文案应含表名和版本: %q", dup)
	}
	if r.Len() != 1 {
		t.Fatalf("重复注册 panic 后 Len 应仍为 1，得到 %d", r.Len())
	}
	if got := r.MustGet("v1"); got.BaseURL != moreBaseURL {
		t.Fatalf("重复注册不得覆盖已有项: %+v", got)
	}

	empty := versionreg.New[emptyIDConfig]("test")
	emptyMsg := mustPanicString(t, func() {
		empty.MustRegister(emptyIDConfig{})
	})
	t.Logf("空 VersionID panic=%q Len=%d", emptyMsg, empty.Len())
	if !strings.Contains(emptyMsg, "test") {
		t.Fatalf("空 VersionID panic 文案应含表名: %q", emptyMsg)
	}
	if empty.Len() != 0 {
		t.Fatalf("空 VersionID panic 后不应写入，Len=%d", empty.Len())
	}
}

var boomValidate = errors.New("boom")

func TestMustRegister_Validate任意错误仍包装可Is(t *testing.T) {
	err := mustPanicError(t, func() {
		r := versionreg.New[boomCfg]("test")
		r.MustRegister(boomCfg{})
	})
	t.Logf("自定义 Validate 错误包装后 %v", err)
	if !errors.Is(err, boomValidate) {
		t.Fatalf("Validate 错误应以 %%w 包装，errors.Is 应命中原因，得到 %v", err)
	}
}

type boomCfg struct{}

func (boomCfg) VersionID() versionreg.ID { return "v1" }
func (boomCfg) Validate() error          { return boomValidate }

func TestMustRegister_指针身份共享实例(t *testing.T) {
	r := newReg(t)
	cfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithUserAgent("ua/1"))
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
	r := versionreg.New[valCfg]("test")
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
	id   versionreg.ID
	mark string
}

func (v valCfg) VersionID() versionreg.ID { return v.id }
func (v valCfg) Validate() error          { return nil }

func TestMustRegister_空白ID可注册(t *testing.T) {
	r := newReg(t)
	r.MustRegister(versionreg.NewConfig("  ", moreBaseURL))
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
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))

	cases := []versionreg.ID{" ", "v1 ", " v1", "V1", "v1\t"}
	for _, id := range cases {
		cfg, err := r.Get(id)
		t.Logf("Get(%q) cfg=%v err=%v", string(id), cfg, err)
		if cfg != nil {
			t.Fatalf("Get(%q) 不应命中 v1（不做 trim / 区分大小写）", string(id))
		}
		assertUnknownVersion(t, err, "test", id, []versionreg.ID{"v1"})
		if r.Has(id) {
			t.Fatalf("Has(%q) 应为 false", string(id))
		}
	}
}

func TestGet_Unknown_Registered是新切片(t *testing.T) {
	r := newReg(t)
	r.MustRegister(versionreg.NewConfig("v2", moreBaseURL))
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))

	_, err := r.Get("v9")
	var unk *versionreg.UnknownVersionError
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
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
	err := mustPanicError(t, func() { r.MustGet("") })
	assertEmptyVersion(t, err, "test", []versionreg.ID{"v1"})
}

func TestMustGet_命中返回同一指针(t *testing.T) {
	r := newReg(t)
	cfg := versionreg.NewConfig("v1", moreBaseURL)
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

	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
	one := r.List()
	t.Logf("单元素 List=%v", one)
	if len(one) != 1 || one[0] != "v1" {
		t.Fatalf("List = %v", one)
	}
}

func TestList_返回切片隔离(t *testing.T) {
	r := newReg(t)
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
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
	for _, id := range []versionreg.ID{"v10", "v2", "v1"} {
		r.MustRegister(versionreg.NewConfig(id, moreBaseURL))
	}
	got := r.List()
	t.Logf("List(%v) = %v", []string{"v10", "v2", "v1"}, got)
	want := []versionreg.ID{"v1", "v10", "v2"}
	if !idSliceEqual(got, want) {
		t.Fatalf("List = %v, want 字典序 %v（不是 semver：v10 应排在 v2 前）", got, want)
	}
}

func TestHas_空与空白与大小写(t *testing.T) {
	r := newReg(t)
	if r.Has("") || r.Has("v1") {
		t.Fatal("空表 Has 应为 false")
	}
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
	cases := []struct {
		id   versionreg.ID
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
	r.MustRegister(versionreg.NewConfig("v1", moreBaseURL))
	if r.Len() != 1 {
		t.Fatalf("注册 1 个后 Len=%d", r.Len())
	}
	r.MustRegister(versionreg.NewConfig("v2", moreBaseURL))
	t.Logf("注册 2 个后 Len=%d", r.Len())
	if r.Len() != 2 {
		t.Fatalf("Len=%d, want 2", r.Len())
	}
}
