package envx

// envx_test.go —— 环境变量读取的全分支覆盖。
// 重点是「非法值回落默认」这条语义：它是刻意的（运维打错一个字母不该让进程起不来），
// 但也绝不能静默变成 0（对超时类配置而言 0 意味着关闭保护），所以每条分支都锁死。

import (
	"testing"
)

// withPrefix 在用例内临时换前缀，结束自动还原（前缀是包级全局状态）。
func withPrefix(t *testing.T, p string) {
	t.Helper()
	old := Prefix()
	SetPrefix(p)
	t.Cleanup(func() { SetPrefix(old) })
}

func TestPrefix_默认值与设置(t *testing.T) {
	if got := Prefix(); got != DefaultPrefix {
		t.Fatalf("默认前缀 = %q, want %q", got, DefaultPrefix)
	}
	withPrefix(t, "MYAPP_")
	if got := Prefix(); got != "MYAPP_" {
		t.Fatalf("Prefix() = %q", got)
	}
	if got := Key("LOG_LEVEL"); got != "MYAPP_LOG_LEVEL" {
		t.Fatalf("Key() = %q", got)
	}
}

func TestPrefix_空前缀用裸名(t *testing.T) {
	withPrefix(t, "")
	if got := Key("LOG_LEVEL"); got != "LOG_LEVEL" {
		t.Fatalf("空前缀应直接用裸名, got %q", got)
	}
}

func TestString_取值与TrimSpace(t *testing.T) {
	withPrefix(t, "T1_")
	t.Setenv("T1_NAME", "  hello  ")
	if got := String("NAME"); got != "hello" {
		t.Fatalf("String() = %q, want 去掉首尾空白", got)
	}
	if got := String("ABSENT"); got != "" {
		t.Fatalf("未设置应返回空串, got %q", got)
	}
}

func TestLower_转小写(t *testing.T) {
	withPrefix(t, "T2_")
	t.Setenv("T2_FORMAT", "  JSON ")
	if got := Lower("FORMAT"); got != "json" {
		t.Fatalf("Lower() = %q", got)
	}
	if got := Lower("ABSENT"); got != "" {
		t.Fatalf("未设置应返回空串, got %q", got)
	}
}

func TestInt_合法值与各类非法值回落(t *testing.T) {
	withPrefix(t, "T3_")
	cases := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{name: "未设置回落默认", set: false, want: 42},
		{name: "合法值", set: true, val: "7", want: 7},
		{name: "零值是合法的", set: true, val: "0", want: 0},
		{name: "空串回落默认", set: true, val: "", want: 42},
		{name: "非数字回落默认", set: true, val: "abc", want: 42},
		{name: "负数回落默认(超时类配置不接受负值)", set: true, val: "-1", want: 42},
		{name: "带空白的合法值", set: true, val: " 9 ", want: 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("T3_N", tc.val)
			} else {
				t.Setenv("T3_N", "")
			}
			if got := Int("N", 42); got != tc.want {
				t.Fatalf("Int() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBool_真值假值与非法值(t *testing.T) {
	withPrefix(t, "T4_")
	cases := []struct {
		val  string
		def  bool
		want bool
	}{
		{"1", false, true},
		{"true", false, true},
		{"TRUE", false, true},
		{"yes", false, true},
		{"on", false, true},
		{"0", true, false},
		{"false", true, false},
		{"no", true, false},
		{"off", true, false},
		{"", true, true},        // 未设置 → 默认值
		{"", false, false},      // 未设置 → 默认值
		{"maybe", true, true},   // 非法 → 默认值
		{"maybe", false, false}, // 非法 → 默认值
	}
	for _, tc := range cases {
		t.Run(tc.val+"/"+boolName(tc.def), func(t *testing.T) {
			t.Setenv("T4_B", tc.val)
			if got := Bool("B", tc.def); got != tc.want {
				t.Fatalf("Bool(%q, %v) = %v, want %v", tc.val, tc.def, got, tc.want)
			}
		})
	}
}

func boolName(b bool) string {
	if b {
		return "def=true"
	}
	return "def=false"
}

func TestSetPrefix_并发读写不炸(t *testing.T) {
	// 前缀是包级全局，SetPrefix 通常只在 main 早期调一次，但读发生在请求热路径上。
	withPrefix(t, "T5_")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			SetPrefix("T5_")
		}
	}()
	for i := 0; i < 200; i++ {
		_ = Key("X")
		_ = String("X")
	}
	<-done
}
