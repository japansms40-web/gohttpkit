package versionreg_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func TestHeaderWhitelists_nil安全(t *testing.T) {
	var w *versionreg.HeaderWhitelists
	gotFor, gotHas, gotEps := w.For("x"), w.Has("x"), w.Endpoints()
	t.Logf("nil For=%v Has=%v Endpoints=%v", gotFor, gotHas, gotEps)
	if gotFor != nil || gotHas || gotEps != nil {
		t.Fatal("nil 白名单集合应安全返回零值")
	}
}

func TestHeaderWhitelists_HasEndpoints与深拷贝(t *testing.T) {
	src := map[versionreg.Endpoint]map[string]string{
		"b.ep": {"accept": ""},
		"a.ep": {"accept": "", "x-k": "v"},
	}
	w := versionreg.NewHeaderWhitelists(src)

	t.Logf("Has(a.ep)=%v Has(zzz)=%v Endpoints=%v", w.Has("a.ep"), w.Has("zzz"), w.Endpoints())
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

	src["a.ep"]["injected"] = "boom"
	if _, leaked := w.For("a.ep")["injected"]; leaked {
		t.Fatal("NewHeaderWhitelists 未做深拷贝")
	}
}

func TestHeaderWhitelists_nil源是空集合不是发全量头的nil指针(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(nil)
	t.Logf("For=%v Has=%v Endpoints=%v len=%d", w.For("x"), w.Has("x"), w.Endpoints(), len(w.Endpoints()))
	if w == nil {
		t.Fatal("NewHeaderWhitelists(nil) 应返回可用的空集合")
	}
	if w.For("x") != nil {
		t.Fatal("空集合 For 应为 nil（不是空 map）")
	}
	if w.Has("x") {
		t.Fatal("空集合 Has 应为 false")
	}
	if w.Endpoints() == nil || len(w.Endpoints()) != 0 {
		t.Fatalf("空集合 Endpoints 长度应为 0，got %v", w.Endpoints())
	}
}
