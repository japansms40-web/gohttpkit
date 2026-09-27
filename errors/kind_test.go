package errors_test

import (
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
)

// sysKind 模拟接入方自己实现的 Kind（外部包实现接口）。
type sysKind string

// Name 实现 Kind。
func (k sysKind) Name() string { return string(k) }

func TestNewKind_名称与可比较(t *testing.T) {
	a := kiterrors.NewKind("demo.login_required")
	if a.Name() != "demo.login_required" {
		t.Fatalf("Name()=%q", a.Name())
	}
	if a != kiterrors.NewKind("demo.login_required") {
		t.Fatal("同名默认 Kind 应相等（值类型，可比较、可作 map 键）")
	}
	if a == kiterrors.NewKind("demo.rate_limited") {
		t.Fatal("不同名 Kind 不应相等")
	}
	m := map[kiterrors.Kind]int{a: 1}
	if m[kiterrors.NewKind("demo.login_required")] != 1 {
		t.Fatal("默认 Kind 应可作 map 键")
	}
	if kiterrors.NewKind("").Name() != "" {
		t.Fatal("空名按字面保留")
	}
}

func TestKind_外部包可自行实现(t *testing.T) {
	var k kiterrors.Kind = sysKind("ext.kind")
	err := &kiterrors.Error{Kind: k}
	if !kiterrors.IsKind(err, sysKind("ext.kind")) || kiterrors.KindOf(err) != k {
		t.Fatalf("外部实现的 Kind 应能判定：KindOf=%v", kiterrors.KindOf(err))
	}
	if kiterrors.IsKind(err, kiterrors.NewKind("ext.kind")) {
		t.Fatal("同名但不同实现类型的 Kind 不应视为同一分类")
	}
}
