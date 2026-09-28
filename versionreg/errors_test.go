package versionreg

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func assertEmptyVersion(t *testing.T, err error, registry string, registered []ID) {
	t.Helper()
	var got *EmptyVersionError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *EmptyVersionError", err, err)
	}
	if got.Registry != registry {
		t.Fatalf("Registry = %q, want %q", got.Registry, registry)
	}
	if !idSliceEqual(got.Registered, registered) {
		t.Fatalf("Registered = %v, want %v", got.Registered, registered)
	}
	t.Logf("errors.As → *EmptyVersionError Registry=%q Registered=%v err=%v", got.Registry, got.Registered, err)
}

func assertUnknownVersion(t *testing.T, err error, registry string, requested ID, registered []ID) {
	t.Helper()
	var got *UnknownVersionError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *UnknownVersionError", err, err)
	}
	if got.Registry != registry {
		t.Fatalf("Registry = %q, want %q", got.Registry, registry)
	}
	if got.Requested != requested {
		t.Fatalf("Requested = %q, want %q", got.Requested, requested)
	}
	if !idSliceEqual(got.Registered, registered) {
		t.Fatalf("Registered = %v, want %v", got.Registered, registered)
	}
	t.Logf("errors.As → *UnknownVersionError Registry=%q Requested=%q Registered=%v err=%v", got.Registry, got.Requested, got.Registered, err)
}

func assertMissingField(t *testing.T, err error, field string) {
	t.Helper()
	var got *MissingConfigFieldError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *MissingConfigFieldError", err, err)
	}
	if got.Field != field {
		t.Fatalf("Field = %q, want %q", got.Field, field)
	}
	t.Logf("errors.As → *MissingConfigFieldError Field=%q err=%v", got.Field, err)
}

func idSliceEqual(a, b []ID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEmptyVersionError_字段与文案(t *testing.T) {
	err := &EmptyVersionError{Registry: "android", Registered: []ID{"v1", "v2"}}
	assertEmptyVersion(t, err, "android", []ID{"v1", "v2"})
	want := "versionreg[android]: 必须指定版本(已注册: [v1 v2])"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestEmptyVersionError_nil接收者(t *testing.T) {
	got := (*EmptyVersionError)(nil).Error()
	t.Logf("(*EmptyVersionError)(nil).Error() = %q", got)
	if got != "versionreg: empty version <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestEmptyVersionError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("pick version: %w", &EmptyVersionError{Registry: "web", Registered: nil})
	t.Logf("wrapped = %v", wrapped)
	assertEmptyVersion(t, wrapped, "web", nil)
}

func TestUnknownVersionError_字段与文案(t *testing.T) {
	err := &UnknownVersionError{Registry: "android", Requested: "v9", Registered: []ID{"v1"}}
	assertUnknownVersion(t, err, "android", "v9", []ID{"v1"})
	want := `versionreg[android]: 不支持的版本 "v9"(已注册: [v1])`
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestUnknownVersionError_nil接收者(t *testing.T) {
	got := (*UnknownVersionError)(nil).Error()
	t.Logf("(*UnknownVersionError)(nil).Error() = %q", got)
	if got != "versionreg: unknown version <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestUnknownVersionError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("load: %w", &UnknownVersionError{Registry: "ios", Requested: "x", Registered: []ID{}})
	t.Logf("wrapped = %v", wrapped)
	assertUnknownVersion(t, wrapped, "ios", "x", []ID{})
}

func TestMissingConfigFieldError_字段与文案(t *testing.T) {
	err := &MissingConfigFieldError{Field: "ID"}
	assertMissingField(t, err, "ID")
	want := "ID 必填"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestMissingConfigFieldError_nil接收者(t *testing.T) {
	got := (*MissingConfigFieldError)(nil).Error()
	t.Logf("(*MissingConfigFieldError)(nil).Error() = %q", got)
	if got != "versionreg: missing config field <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestMissingConfigFieldError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("validate: %w", &MissingConfigFieldError{Field: "BaseURL"})
	t.Logf("wrapped = %v", wrapped)
	assertMissingField(t, wrapped, "BaseURL")
}

func TestVersionreg错误类型互不误匹配(t *testing.T) {
	empty := &EmptyVersionError{Registry: "t"}
	unknown := &UnknownVersionError{Registry: "t", Requested: "v9"}
	missing := &MissingConfigFieldError{Field: "ID"}

	var asEmpty *EmptyVersionError
	var asUnknown *UnknownVersionError
	var asMissing *MissingConfigFieldError
	if errors.As(unknown, &asEmpty) || errors.As(missing, &asEmpty) {
		t.Fatal("非空版本错误不应被 As 成 *EmptyVersionError")
	}
	if errors.As(empty, &asUnknown) || errors.As(missing, &asUnknown) {
		t.Fatal("非未知版本错误不应被 As 成 *UnknownVersionError")
	}
	if errors.As(empty, &asMissing) || errors.As(unknown, &asMissing) {
		t.Fatal("非缺字段错误不应被 As 成 *MissingConfigFieldError")
	}
	t.Logf("三类错误互不误匹配")
}

func TestGet_空版本与未知版本是类型错误且Registered字典序新切片(t *testing.T) {
	r := New[*Config]("test")
	r.MustRegister(NewConfig("v2", "https://a.example"))
	r.MustRegister(NewConfig("v1", "https://a.example"))

	_, err := r.Get("")
	t.Logf("Get(\"\") → %v", err)
	assertEmptyVersion(t, err, "test", []ID{"v1", "v2"})
	var empty *EmptyVersionError
	errors.As(err, &empty)
	empty.Registered[0] = "mutated"
	if got := r.List(); got[0] != "v1" {
		t.Fatal("Registered 必须是新切片，改错误字段不得污染注册表")
	}

	_, err = r.Get("v9")
	t.Logf("Get(\"v9\") → %v", err)
	assertUnknownVersion(t, err, "test", "v9", []ID{"v1", "v2"})
}

func TestConfig_Validate缺字段是MissingConfigField(t *testing.T) {
	err := NewConfig("", "https://a.example").Validate()
	t.Logf("缺 ID → %v", err)
	assertMissingField(t, err, "ID")

	err = NewConfig("v1", "").Validate()
	t.Logf("缺 BaseURL → %v", err)
	assertMissingField(t, err, "BaseURL")
}

func TestMustGet_panic保留类型错误(t *testing.T) {
	defer func() {
		r := recover()
		t.Logf("recover=%v (%T)", r, r)
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic 值应是 error，得到 %T", r)
		}
		assertUnknownVersion(t, err, "test", "nope", nil)
	}()
	New[*Config]("test").MustGet("nope")
}

func TestMustRegister_Validate失败panic可As到缺字段(t *testing.T) {
	defer func() {
		r := recover()
		t.Logf("recover=%v (%T)", r, r)
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic 值应是 error，得到 %T", r)
		}
		assertMissingField(t, err, "BaseURL")
	}()
	New[*Config]("test").MustRegister(NewConfig("v1", ""))
}

func Test错误结构体不含内部map引用字段(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(EmptyVersionError{}),
		reflect.TypeOf(UnknownVersionError{}),
		reflect.TypeOf(MissingConfigFieldError{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.Map {
				t.Errorf("%s.%s 不得引用内部 map", typ.Name(), f.Name)
			}
		}
	}
}

// —— 角度补充：三类类型错误 Error() 的边界 / 零值 / 契约。
// 上方用例已锁典型字段、nil 接收者、包装后 As、互不误匹配。
// 这里补空 Registry / 空 Registered / 空 Field，以及 Get 失败返回零值。
// 比的是 Error() 契约文案本身，不是拿文案当错误身份。

func TestEmptyVersionError_零值字段文案(t *testing.T) {
	err := &EmptyVersionError{}
	got := err.Error()
	t.Logf("零值 Error() = %q", got)
	want := "versionreg[]: 必须指定版本(已注册: [])"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	assertEmptyVersion(t, err, "", nil)
}

func TestEmptyVersionError_空Registered与显式空切片(t *testing.T) {
	nilReg := &EmptyVersionError{Registry: "web", Registered: nil}
	emptyReg := &EmptyVersionError{Registry: "web", Registered: []ID{}}
	t.Logf("Registered=nil → %q ; Registered=[] → %q", nilReg.Error(), emptyReg.Error())
	assertEmptyVersion(t, nilReg, "web", nil)
	assertEmptyVersion(t, emptyReg, "web", []ID{})
	if nilReg.Error() != emptyReg.Error() {
		t.Fatal("nil 与空切片 Registered 的 Error() 应同形")
	}
}

func TestUnknownVersionError_零值字段文案(t *testing.T) {
	err := &UnknownVersionError{}
	got := err.Error()
	t.Logf("零值 Error() = %q", got)
	want := `versionreg[]: 不支持的版本 ""(已注册: [])`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	assertUnknownVersion(t, err, "", "", nil)
}

func TestUnknownVersionError_空Requested(t *testing.T) {
	err := &UnknownVersionError{Registry: "ios", Requested: "", Registered: []ID{"v1"}}
	t.Logf("空 Requested Error() = %q", err.Error())
	assertUnknownVersion(t, err, "ios", "", []ID{"v1"})
	want := `versionreg[ios]: 不支持的版本 ""(已注册: [v1])`
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestMissingConfigFieldError_空Field(t *testing.T) {
	err := &MissingConfigFieldError{}
	got := err.Error()
	t.Logf("空 Field Error() = %q", got)
	if got != " 必填" {
		t.Errorf("Error() = %q, want %q", got, " 必填")
	}
	assertMissingField(t, err, "")
}

func TestMissingConfigFieldError_其它字段名(t *testing.T) {
	err := &MissingConfigFieldError{Field: "BaseURL"}
	t.Logf("Field=BaseURL Error() = %q", err.Error())
	assertMissingField(t, err, "BaseURL")
	if err.Error() != "BaseURL 必填" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestGet失败返回零值T(t *testing.T) {
	r := New[*Config]("test")
	for _, id := range []ID{"", "nope"} {
		cfg, err := r.Get(id)
		t.Logf("Get(%q) cfg=%v err=%v", string(id), cfg, err)
		if cfg != nil {
			t.Fatalf("Get(%q) 失败应返回 nil 指针，得到 %p", string(id), cfg)
		}
		if err == nil {
			t.Fatalf("Get(%q) 应失败", string(id))
		}
	}
}

func Test错误As解出的是同一指针不是副本(t *testing.T) {
	inner := &UnknownVersionError{
		Registry:   "t",
		Requested:  "v9",
		Registered: []ID{"v1", "v2"},
	}
	var first *UnknownVersionError
	if !errors.As(inner, &first) {
		t.Fatal("As 应命中")
	}
	first.Registered[0] = "mutated"
	var second *UnknownVersionError
	if !errors.As(inner, &second) {
		t.Fatal("第二次 As 应仍命中")
	}
	t.Logf("改 first.Registered 后 second.Registered=%v", second.Registered)
	if second.Registered[0] != "mutated" {
		t.Fatal("errors.As 解出的是同一指针，改字段应可见（与 Get 返回的新切片不同）")
	}
}
