package versionreg_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func assertEmptyVersion(t *testing.T, err error, registry string, registered []versionreg.ID) {
	t.Helper()
	var got *versionreg.EmptyVersionError
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

func assertUnknownVersion(t *testing.T, err error, registry string, requested versionreg.ID, registered []versionreg.ID) {
	t.Helper()
	var got *versionreg.UnknownVersionError
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
	var got *versionreg.MissingConfigFieldError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *MissingConfigFieldError", err, err)
	}
	if got.Field != field {
		t.Fatalf("Field = %q, want %q", got.Field, field)
	}
	t.Logf("errors.As → *MissingConfigFieldError Field=%q err=%v", got.Field, err)
}

func idSliceEqual(a, b []versionreg.ID) bool {
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
	err := &versionreg.EmptyVersionError{Registry: "android", Registered: []versionreg.ID{"v1", "v2"}}
	assertEmptyVersion(t, err, "android", []versionreg.ID{"v1", "v2"})
	want := "versionreg[android]: 必须指定版本(已注册: [v1 v2])"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestEmptyVersionError_nil接收者(t *testing.T) {
	got := (*versionreg.EmptyVersionError)(nil).Error()
	t.Logf("(*EmptyVersionError)(nil).Error() = %q", got)
	if got != "versionreg: empty version <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestEmptyVersionError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("pick version: %w", &versionreg.EmptyVersionError{Registry: "web", Registered: nil})
	t.Logf("wrapped = %v", wrapped)
	assertEmptyVersion(t, wrapped, "web", nil)
}

func TestUnknownVersionError_字段与文案(t *testing.T) {
	err := &versionreg.UnknownVersionError{Registry: "android", Requested: "v9", Registered: []versionreg.ID{"v1"}}
	assertUnknownVersion(t, err, "android", "v9", []versionreg.ID{"v1"})
	want := `versionreg[android]: 不支持的版本 "v9"(已注册: [v1])`
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestUnknownVersionError_nil接收者(t *testing.T) {
	got := (*versionreg.UnknownVersionError)(nil).Error()
	t.Logf("(*UnknownVersionError)(nil).Error() = %q", got)
	if got != "versionreg: unknown version <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestUnknownVersionError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("load: %w", &versionreg.UnknownVersionError{Registry: "ios", Requested: "x", Registered: []versionreg.ID{}})
	t.Logf("wrapped = %v", wrapped)
	assertUnknownVersion(t, wrapped, "ios", "x", []versionreg.ID{})
}

func TestMissingConfigFieldError_字段与文案(t *testing.T) {
	err := &versionreg.MissingConfigFieldError{Field: "ID"}
	assertMissingField(t, err, "ID")
	want := "ID 必填"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestMissingConfigFieldError_nil接收者(t *testing.T) {
	got := (*versionreg.MissingConfigFieldError)(nil).Error()
	t.Logf("(*MissingConfigFieldError)(nil).Error() = %q", got)
	if got != "versionreg: missing config field <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestMissingConfigFieldError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("validate: %w", &versionreg.MissingConfigFieldError{Field: "BaseURL"})
	t.Logf("wrapped = %v", wrapped)
	assertMissingField(t, wrapped, "BaseURL")
}

func TestVersionreg错误类型互不误匹配(t *testing.T) {
	empty := &versionreg.EmptyVersionError{Registry: "t"}
	unknown := &versionreg.UnknownVersionError{Registry: "t", Requested: "v9"}
	missing := &versionreg.MissingConfigFieldError{Field: "ID"}

	var asEmpty *versionreg.EmptyVersionError
	var asUnknown *versionreg.UnknownVersionError
	var asMissing *versionreg.MissingConfigFieldError
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
	r := versionreg.New[*versionreg.Config]("test")
	r.MustRegister(versionreg.NewConfig("v2", "https://a.example"))
	r.MustRegister(versionreg.NewConfig("v1", "https://a.example"))

	_, err := r.Get("")
	t.Logf("Get(\"\") → %v", err)
	assertEmptyVersion(t, err, "test", []versionreg.ID{"v1", "v2"})
	var empty *versionreg.EmptyVersionError
	errors.As(err, &empty)
	empty.Registered[0] = "mutated"
	if got := r.List(); got[0] != "v1" {
		t.Fatal("Registered 必须是新切片，改错误字段不得污染注册表")
	}

	_, err = r.Get("v9")
	t.Logf("Get(\"v9\") → %v", err)
	assertUnknownVersion(t, err, "test", "v9", []versionreg.ID{"v1", "v2"})
}

func TestConfig_Validate缺字段是MissingConfigField(t *testing.T) {
	err := versionreg.NewConfig("", "https://a.example").Validate()
	t.Logf("缺 ID → %v", err)
	assertMissingField(t, err, "ID")

	err = versionreg.NewConfig("v1", "").Validate()
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
	versionreg.New[*versionreg.Config]("test").MustGet("nope")
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
	versionreg.New[*versionreg.Config]("test").MustRegister(versionreg.NewConfig("v1", ""))
}

func Test错误结构体不含内部map引用字段(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(versionreg.EmptyVersionError{}),
		reflect.TypeOf(versionreg.UnknownVersionError{}),
		reflect.TypeOf(versionreg.MissingConfigFieldError{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.Map {
				t.Errorf("%s.%s 不得引用内部 map", typ.Name(), f.Name)
			}
		}
	}
}
