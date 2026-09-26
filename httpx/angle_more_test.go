package httpx_test

// angle_more_test.go —— 纯函数边界、marker 方法体、DefaultStatusRule、WithChain(nil)。

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
	"unsafe"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/netproxy"
)

func TestMarker方法体可直接调用(t *testing.T) {
	httpx.SideChannelMarker{}.SideChannel()
	httpx.TerminalMarker{}.Terminal()
	t.Logf("SideChannel/Terminal 空方法可调用、无副作用")
	if httpx.IsSideChannel(nil) {
		t.Fatal("IsSideChannel(nil) 应为 false")
	}
}

func TestDefaultStatusRule_空体与有体(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    []byte
		wantErr bool
	}{
		{"空体404", 404, nil, true},
		{"空切片404", 404, []byte{}, true},
		{"1字节放行", 400, []byte("{"), false},
		{"有体放行", 500, []byte(`{"e":1}`), false},
		{"2xx空体仍报错(纯函数不管谁调用)", 200, nil, true},
	}
	for _, c := range cases {
		err := httpx.DefaultStatusRule(c.status, c.body)
		t.Logf("%s status=%d len(body)=%d err=%v", c.name, c.status, len(c.body), err)
		if !c.wantErr {
			if err != nil {
				t.Errorf("%s: 有体应放行，got %v", c.name, err)
			}
			continue
		}
		var se *kiterrors.HTTPStatusError
		if !errors.As(err, &se) || se.StatusCode != c.status {
			t.Errorf("%s: 要 *HTTPStatusError{%d}，got %v", c.name, c.status, err)
		}
	}
}

func TestRetryableTextRule_空体命中状态码As字段(t *testing.T) {
	rule := httpx.RetryableTextRule(nil, 503)
	err := rule(503, nil)
	t.Logf("503 空体 → %v", err)
	var re *kiterrors.RetryableError
	if !errors.As(err, &re) {
		t.Fatalf("要 *RetryableError，got %v", err)
	}
	var se *kiterrors.HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 503 {
		t.Fatalf("应包装 HTTPStatusError(503)，got %v", err)
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

func TestEncodeRequestBody_nil与字节同引用与string与表单(t *testing.T) {
	got, err := httpx.EncodeRequestBody(nil)
	t.Logf("nil → %v err=%v", got, err)
	if got != nil || err != nil {
		t.Fatalf("nil → (%v, %v)", got, err)
	}

	in := []byte("hi")
	got, err = httpx.EncodeRequestBody(in)
	if err != nil {
		t.Fatal(err)
	}
	if unsafe.SliceData(got) != unsafe.SliceData(in) {
		t.Fatal("[]byte 应返回同一份切片，不复制")
	}

	got, err = httpx.EncodeRequestBody("a=1&b=2")
	t.Logf("string → %q", got)
	if err != nil || string(got) != "a=1&b=2" {
		t.Fatalf("string 应原样，got %q err=%v", got, err)
	}

	got, err = httpx.EncodeRequestBody(url.Values{"b": {"2"}, "a": {"1"}})
	t.Logf("url.Values → %q", got)
	if err != nil || string(got) != "a=1&b=2" {
		t.Fatalf("url.Values 应按字典序，got %q", got)
	}
}

func TestDecodeResponse_空与非指针(t *testing.T) {
	var m map[string]any
	err := httpx.DecodeResponse(nil, &m)
	t.Logf("nil data → err=%v", err)
	var de *httpx.ResponseJSONDecodeError
	if !errors.As(err, &de) {
		t.Fatalf("空输入要 *ResponseJSONDecodeError，got %v", err)
	}

	var val map[string]any
	err = httpx.DecodeResponse([]byte(`{"a":1}`), val)
	t.Logf("非指针 → err=%v TargetType=%s", err, "")
	if !errors.As(err, &de) {
		t.Fatalf("非指针要 *ResponseJSONDecodeError，got %v", err)
	}
}

func TestTruncateBodyForLog_nil体(t *testing.T) {
	got := httpx.TruncateBodyForLog(nil, 3)
	t.Logf("nil → %v nil=%v", got, got == nil)
	if got != nil {
		t.Fatalf("nil 体应原样返回 nil，got %v", got)
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

func TestWithChain_nil只保留最外层旁路(t *testing.T) {
	parent, err := httpx.NewClient(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://a.example"},
		Interceptors: httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	child := parent.WithChain(nil)
	got := child.Interceptors()
	t.Logf("WithChain(nil) len=%d", len(got))
	if len(got) != 1 {
		t.Fatalf("默认链最外层只有 transaction 是 SideChannel，len=%d", len(got))
	}
	if !httpx.IsSideChannel(got[0]) {
		t.Fatal("继承层应是 SideChannel")
	}
}

func TestSlowMS_亚毫秒截断为0(t *testing.T) {
	c, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://a.example"},
		SlowMS:  500 * time.Microsecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("500µs → SlowMS()=%d", c.SlowMS())
	if c.SlowMS() != 0 {
		t.Fatalf("不足 1ms 应截断为 0（关闭），got %d", c.SlowMS())
	}
}

func TestCacheResponseHeaders_nil清空(t *testing.T) {
	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://a.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.SnapshotResponseHeaders() != nil || c.SnapshotResponseStatusCode() != 0 {
		t.Fatal("New 后快照应为零值")
	}
	c.CacheResponseHeaders(map[string][]string{"x": {"1"}})
	c.CacheResponseHeaders(nil)
	t.Logf("清空后 Snapshot=%v", c.SnapshotResponseHeaders())
	if c.SnapshotResponseHeaders() != nil {
		t.Fatal("CacheResponseHeaders(nil) 应清空")
	}
}

func TestNewTransport_ftp是UnsupportedScheme(t *testing.T) {
	_, err := httpx.NewTransport("ftp://h:1")
	t.Logf("ftp → %v", err)
	var ue *netproxy.UnsupportedProxySchemeError
	if !errors.As(err, &ue) || ue.Scheme != "ftp" {
		t.Fatalf("要 *UnsupportedProxySchemeError{ftp}，got %v", err)
	}
}

func TestPrepend_空items是新切片(t *testing.T) {
	base := httpx.Interceptors{interceptor.NewLoggingInterceptor()}
	got := httpx.Prepend(base)
	t.Logf("Prepend(base) len=%d", len(got))
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	got[0] = interceptor.NewRetryInterceptor()
	if base[0] == got[0] {
		t.Fatal("Prepend 应新切片，改 got 不得写回 base")
	}
}

func TestExtractHTMLText_畸形不panic(t *testing.T) {
	in := "<<<>}{{{not-html"
	got := httpx.ExtractHTMLText(in)
	t.Logf("畸形 → %q", got)
	// x/net/html 解析器极宽，几乎不返回 error；此用例锁「不 panic、有确定输出」。
	_ = got
}
