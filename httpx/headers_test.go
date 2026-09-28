package httpx_test

// headers_test.go —— HeaderProvider 与白名单过滤的单元契约。

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────────────── headers.go ───────────────────────────────

func TestStaticHeaders_返回副本且key转小写(t *testing.T) {
	sh := httpx.StaticHeaders{Base: "https://x.example", Headers: map[string]string{"Accept": "*/*"}}
	got := sh.BuildHeaders(t.Context())
	t.Logf("BuildHeaders=%v BaseURL=%q", got, sh.BaseURL())
	if got["accept"] != "*/*" {
		t.Fatalf("key 未转小写: %v", got)
	}
	got["injected"] = "boom"
	if _, leaked := sh.BuildHeaders(t.Context())["injected"]; leaked {
		t.Fatal("StaticHeaders 返回了内部 map")
	}
	if sh.BaseURL() != "https://x.example" {
		t.Fatal("BaseURL 不对")
	}
}

func TestHeaderProviderFunc_Build为nil时返回空表而非nil(t *testing.T) {
	// 返回 nil 会被 bridge 判为「构头失败」，一个没配 Build 的 provider
	// 不该表现成构头失败，而该表现成「没有任何头」。
	f := httpx.HeaderProviderFunc{Base: "https://x.example"}
	got := f.BuildHeaders(t.Context())
	t.Logf("Build=%v nil=%v BaseURL=%q", got, got == nil, f.BaseURL())
	if got == nil {
		t.Fatal("应返回空 map 而不是 nil")
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if f.BaseURL() != "https://x.example" {
		t.Fatal("BaseURL 不对")
	}
}

func TestFilterHeadersByWhitelist_空白名单返回空表(t *testing.T) {
	got := httpx.FilterHeadersByWhitelist(map[string]string{"a": "1"}, nil)
	t.Logf("whitelist=nil → %v", got)
	if got == nil || len(got) != 0 {
		t.Fatalf("got %v, want 空表", got)
	}
}

func TestHeaderIsHTML(t *testing.T) {
	cases := []struct {
		name string
		h    http.Header
		want bool
	}{
		{name: "nil", h: nil, want: false},
		{name: "json", h: http.Header{"Content-Type": []string{"application/json"}}, want: false},
		{name: "html", h: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httpx.HeaderIsHTML(tc.h)
			t.Logf("HeaderIsHTML(%v) = %v", tc.h, got)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSnapshotRequestHeaders_nil与非首次尝试时不动(t *testing.T) {
	httpx.SnapshotRequestHeaders(nil)
	t.Logf("nil snapshot ok")
	req := &httpx.Request{Attempt: 1}
	httpx.SnapshotRequestHeaders(req)
	t.Logf("attempt=1 headers=%v", req.ReqHeaders)
	if req.ReqHeaders != nil {
		t.Fatal("非首次尝试不该覆盖快照")
	}
	req2 := &httpx.Request{Attempt: 0}
	httpx.SnapshotRequestHeaders(req2)
	t.Logf("HTTPReq=nil headers=%v", req2.ReqHeaders)
	if req2.ReqHeaders != nil {
		t.Fatal("HTTPReq 为 nil 时不该写快照")
	}
}

func TestSnapshotRequestHeaders_无Host时回落URL(t *testing.T) {
	u, _ := url.Parse("https://fallback.example/x")
	req := &httpx.Request{
		Attempt: 0,
		HTTPReq: &http.Request{URL: u, Header: http.Header{"accept": []string{"*/*"}}},
	}
	httpx.SnapshotRequestHeaders(req)
	//nolint:staticcheck // SA1008：本库刻意用全小写 key（真实客户端在 HTTP/2 上发的就是小写），
	// 快照沿用同一约定，这里断言的正是这个契约本身
	got := req.ReqHeaders["host"]
	t.Logf("host=%v", got)
	if len(got) != 1 || got[0] != "fallback.example" {
		t.Fatalf("host = %v, want 从 URL 回落", got)
	}
}

func TestBuildOriginAndReferer(t *testing.T) {
	o, r := httpx.BuildOriginAndReferer("https://x.example", "/a/b")
	t.Logf("base=%q path=%q → origin=%q referer=%q", "https://x.example", "/a/b", o, r)
	if o != "https://x.example" || r != "https://x.example/a/b" {
		t.Fatalf("origin=%q referer=%q", o, r)
	}
}

func TestFilterHeadersByWhitelist_固定值与大小写回退与缺键(t *testing.T) {
	all := map[string]string{"Accept": "*/*", "x-app": "1"}
	got := httpx.FilterHeadersByWhitelist(all, map[string]string{
		"accept":  "",
		"x-app":   "fixed",
		"missing": "",
	})
	t.Logf("filtered=%v", got)
	if got["accept"] != "*/*" {
		t.Fatalf("空值应回退大小写命中 Accept，got %q", got["accept"])
	}
	if got["x-app"] != "fixed" {
		t.Fatalf("非空白名单值应覆盖，got %q", got["x-app"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("allHeaders 没有的键应跳过")
	}

	nilAll := httpx.FilterHeadersByWhitelist(nil, map[string]string{"a": "1"})
	t.Logf("all=nil 固定值 → %v", nilAll)
	if nilAll["a"] != "1" {
		t.Fatalf("all=nil 时固定值仍应写入，got %v", nilAll)
	}
}

func TestBuildOriginAndReferer_空边界(t *testing.T) {
	o, r := httpx.BuildOriginAndReferer("", "")
	t.Logf("空 → origin=%q referer=%q", o, r)
	if o != "" || r != "" {
		t.Fatalf("空 base+path 应都是空串，got %q %q", o, r)
	}
	o, r = httpx.BuildOriginAndReferer("https://a.example", "")
	if o != "https://a.example" || r != "https://a.example" {
		t.Fatalf("空 path → %q %q", o, r)
	}
}

func TestStaticHeaders_Headers为nil是空表(t *testing.T) {
	got := httpx.StaticHeaders{Base: "https://a.example"}.BuildHeaders(t.Context())
	t.Logf("Headers=nil → %v nil=%v", got, got == nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("应为空 map，got %v", got)
	}
}

func TestHeaderProviderFunc_Build返回nil原样nil(t *testing.T) {
	f := httpx.HeaderProviderFunc{
		Base:  "https://a.example",
		Build: func(context.Context) map[string]string { return nil },
	}
	got := f.BuildHeaders(t.Context())
	t.Logf("Build=nil 函数 → %v", got)
	if got != nil {
		t.Fatal("显式返回 nil 应原样（bridge 会当成构头失败）")
	}
}
