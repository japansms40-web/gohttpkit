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
	got := sh.BuildHeaders(context.Background())
	t.Logf("BuildHeaders=%v BaseURL=%q", got, sh.BaseURL())
	if got["accept"] != "*/*" {
		t.Fatalf("key 未转小写: %v", got)
	}
	got["injected"] = "boom"
	if _, leaked := sh.BuildHeaders(context.Background())["injected"]; leaked {
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
	got := f.BuildHeaders(context.Background())
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
