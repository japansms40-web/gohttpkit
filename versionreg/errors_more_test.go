package versionreg_test

// errors_more_test.go —— 三类类型错误 Error() 的边界 / 零值 / 契约。
// 现有 errors_test.go 已锁典型字段、nil 接收者、包装后 As、互不误匹配。
// 这里补空 Registry / 空 Registered / 空 Field，以及 Get 失败返回零值。
// 比的是 Error() 契约文案本身，不是拿文案当错误身份。

import (
	"errors"
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func TestEmptyVersionError_零值字段文案(t *testing.T) {
	err := &versionreg.EmptyVersionError{}
	got := err.Error()
	t.Logf("零值 Error() = %q", got)
	want := "versionreg[]: 必须指定版本(已注册: [])"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	assertEmptyVersion(t, err, "", nil)
}

func TestEmptyVersionError_空Registered与显式空切片(t *testing.T) {
	nilReg := &versionreg.EmptyVersionError{Registry: "web", Registered: nil}
	emptyReg := &versionreg.EmptyVersionError{Registry: "web", Registered: []versionreg.ID{}}
	t.Logf("Registered=nil → %q ; Registered=[] → %q", nilReg.Error(), emptyReg.Error())
	assertEmptyVersion(t, nilReg, "web", nil)
	assertEmptyVersion(t, emptyReg, "web", []versionreg.ID{})
	if nilReg.Error() != emptyReg.Error() {
		t.Fatal("nil 与空切片 Registered 的 Error() 应同形")
	}
}

func TestUnknownVersionError_零值字段文案(t *testing.T) {
	err := &versionreg.UnknownVersionError{}
	got := err.Error()
	t.Logf("零值 Error() = %q", got)
	want := `versionreg[]: 不支持的版本 ""(已注册: [])`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	assertUnknownVersion(t, err, "", "", nil)
}

func TestUnknownVersionError_空Requested(t *testing.T) {
	err := &versionreg.UnknownVersionError{Registry: "ios", Requested: "", Registered: []versionreg.ID{"v1"}}
	t.Logf("空 Requested Error() = %q", err.Error())
	assertUnknownVersion(t, err, "ios", "", []versionreg.ID{"v1"})
	want := `versionreg[ios]: 不支持的版本 ""(已注册: [v1])`
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestMissingConfigFieldError_空Field(t *testing.T) {
	err := &versionreg.MissingConfigFieldError{}
	got := err.Error()
	t.Logf("空 Field Error() = %q", got)
	if got != " 必填" {
		t.Errorf("Error() = %q, want %q", got, " 必填")
	}
	assertMissingField(t, err, "")
}

func TestMissingConfigFieldError_其它字段名(t *testing.T) {
	err := &versionreg.MissingConfigFieldError{Field: "BaseURL"}
	t.Logf("Field=BaseURL Error() = %q", err.Error())
	assertMissingField(t, err, "BaseURL")
	if err.Error() != "BaseURL 必填" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestGet失败返回零值T(t *testing.T) {
	r := versionreg.New[*versionreg.Config]("test")
	for _, id := range []versionreg.ID{"", "nope"} {
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
	inner := &versionreg.UnknownVersionError{
		Registry:   "t",
		Requested:  "v9",
		Registered: []versionreg.ID{"v1", "v2"},
	}
	var first *versionreg.UnknownVersionError
	if !errors.As(inner, &first) {
		t.Fatal("As 应命中")
	}
	first.Registered[0] = "mutated"
	var second *versionreg.UnknownVersionError
	if !errors.As(inner, &second) {
		t.Fatal("第二次 As 应仍命中")
	}
	t.Logf("改 first.Registered 后 second.Registered=%v", second.Registered)
	if second.Registered[0] != "mutated" {
		t.Fatal("errors.As 解出的是同一指针，改字段应可见（与 Get 返回的新切片不同）")
	}
}
