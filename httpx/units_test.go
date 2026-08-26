package httpx_test

// units_test.go —— API 面的单元覆盖：编解码、选项归一化、链编辑工具、
// 各类 nil / 零值 / 错误分支。characterization_test.go 管「端到端行为」，
// 这里管「每个导出符号自己的契约」。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────────────── body.go ───────────────────────────────

func TestEncodeRequestBody_struct走JSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	got, err := httpx.EncodeRequestBody(payload{Name: "x", N: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"x","n":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeRequestBody_不可序列化类型报错(t *testing.T) {
	_, err := httpx.EncodeRequestBody(make(chan int))
	if err == nil || !strings.Contains(err.Error(), "JSON 编码失败") {
		t.Fatalf("err = %v, want 明确的编码失败提示", err)
	}
}

func TestDecodeResponse(t *testing.T) {
	var out struct {
		OK bool `json:"ok"`
	}
	if err := httpx.DecodeResponse([]byte(`{"ok":true}`), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("未解析出字段")
	}
	if err := httpx.DecodeResponse([]byte(`not json`), &out); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestTruncateBodyForLog_边界(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"短于上限原样", "abc", 10, "abc"},
		{"等于上限原样", "abc", 3, "abc"},
		{"超出则截断加占位", "abcdef", 3, "abc...(truncated, total=6)"},
		{"limit 为 0 不截断", "abcdef", 0, "abcdef"},
		{"limit 为负不截断", "abcdef", -1, "abcdef"},
		{"空体", "", 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(httpx.TruncateBodyForLog([]byte(tc.in), tc.limit)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────── headers.go ───────────────────────────────

func TestStaticHeaders_返回副本且key转小写(t *testing.T) {
	sh := httpx.StaticHeaders{Base: "https://x.example", Headers: map[string]string{"Accept": "*/*"}}
	got := sh.BuildHeaders(context.Background())
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
	if got == nil || len(got) != 0 {
		t.Fatalf("got %v, want 空表", got)
	}
}

func TestBuildOriginAndReferer(t *testing.T) {
	o, r := httpx.BuildOriginAndReferer("https://x.example", "/a/b")
	if o != "https://x.example" || r != "https://x.example/a/b" {
		t.Fatalf("origin=%q referer=%q", o, r)
	}
}

// ─────────────────────────────── html.go ───────────────────────────────

func TestExtractHTMLText_忽略脚本样式并压缩空白(t *testing.T) {
	in := `<html><head><style>p{color:red}</style><script>var a=1</script></head>
	       <body><p>  Hello  </p><noscript>NO</noscript><iframe>IF</iframe><p>World</p></body></html>`
	if got := httpx.ExtractHTMLText(in); got != "Hello World" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractHTMLText_空输入(t *testing.T) {
	if got := httpx.ExtractHTMLText(""); got != "" {
		t.Fatalf("got %q", got)
	}
}

// ─────────────────────────────── chain.go ───────────────────────────────

func TestTransportError_文案与解包(t *testing.T) {
	inner := errors.New("boom")
	te := &httpx.TransportError{Err: inner}
	if te.Error() != "boom" {
		t.Fatalf("Error() = %q", te.Error())
	}
	if !errors.Is(te, inner) {
		t.Fatal("应支持 errors.Is 解包")
	}
}

func TestSideChannelMarker_可用于自定义旁路层(t *testing.T) {
	var it httpx.Interceptor = &customObserver{}
	if _, ok := it.(httpx.SideChannel); !ok {
		t.Fatal("内嵌 SideChannelMarker 后应满足 SideChannel 接口")
	}
	if !httpx.IsSideChannel(it) {
		t.Fatal("IsSideChannel 应识别它")
	}
	it.(httpx.SideChannel).SideChannel() // 标记方法本身可调用
}

type customObserver struct {
	httpx.SideChannelMarker
	seen int
}

func (o *customObserver) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	o.seen++
	return resp, err
}

func TestChain_访问器返回请求与客户端(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	var gotPath string
	var sameClient bool
	c := newClient(t, srv.Server, nil)
	probe := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		gotPath = ch.Request().Path
		sameClient = ch.Client() == c
		return ch.Proceed()
	})
	c2, err := httpx.New(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Interceptors: httpx.Prepend(httpx.DefaultChain(), probe),
	})
	if err != nil {
		t.Fatal(err)
	}
	c = c2
	if _, err := c.Get(context.Background(), "/probe", nil); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/probe" {
		t.Fatalf("Request().Path = %q", gotPath)
	}
	if !sameClient {
		t.Fatal("Client() 应返回发起请求的那个客户端")
	}
}

// ─────────────────────────────── client.go ───────────────────────────────

func TestPostJSON_自动带contentType(t *testing.T) {
	var gotCT, gotBody string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.PostJSON(context.Background(), "/x", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	gotBody = string(srv.bodies[len(srv.bodies)-1])
	srv.mu <- struct{}{}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}
	if gotBody != `{"n":1}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestClient_访问器(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	hp := httpx.StaticHeaders{Base: srv.URL, Headers: map[string]string{"accept": "*/*"}}
	c, err := httpx.New(httpx.Options{Headers: hp, ProxyURL: "", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.Headers().BaseURL() != srv.URL {
		t.Fatal("Headers() 不对")
	}
	if got := len(c.Interceptors()); got != len(httpx.DefaultChain()) {
		t.Fatalf("Interceptors() 长度 = %d", got)
	}
	// 返回的是副本：改它不该影响客户端
	chain := c.Interceptors()
	chain[0] = nil
	if c.Interceptors()[0] == nil {
		t.Fatal("Interceptors() 返回了内部切片")
	}
	if c.Options().Timeout != 5*time.Second {
		t.Fatal("Options() 不对")
	}
	if c.RetryPolicy().MaxRetries != 3 {
		t.Fatalf("默认重试次数 = %d, want 3", c.RetryPolicy().MaxRetries)
	}
	if c.HTTPClient.Timeout != 5*time.Second {
		t.Fatalf("Timeout 未生效: %v", c.HTTPClient.Timeout)
	}
	c.SetTimeout(time.Second)
	if c.HTTPClient.Timeout != time.Second {
		t.Fatal("SetTimeout 未生效")
	}
}

func TestLogBodyLimit_三种情形(t *testing.T) {
	var nilClient *httpx.Client
	if got := nilClient.LogBodyLimit(); got != httpx.LogBodyMaxBytes {
		t.Fatalf("nil client = %d, want 回落默认(截断开启)", got)
	}

	c, err := httpx.New(httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.LogBodyLimit(); got != httpx.LogBodyMaxBytes {
		t.Fatalf("默认 = %d", got)
	}

	c2, err := httpx.New(httpx.Options{
		Headers:                  httpx.StaticHeaders{Base: "https://x.example"},
		DisableLogBodyTruncation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := c2.LogBodyLimit(); got != 0 {
		t.Fatalf("关闭截断时 = %d, want 0", got)
	}
}

func TestDo_body编码失败时不发请求(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(context.Background(), httpx.RequestSpec{Method: http.MethodPost, Path: "/x", Body: make(chan int)})
	if err == nil {
		t.Fatal("want error")
	}
	if srv.count() != 0 {
		t.Fatal("编码失败不该发出网络请求")
	}
}

func TestDo_空Method默认GET(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	if _, err := c.Do(context.Background(), httpx.RequestSpec{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if got := srv.last().Method; got != http.MethodGet {
		t.Fatalf("method = %q", got)
	}
}

func TestDo_空链回落默认链(t *testing.T) {
	// 显式传空切片（不是 nil）时，New 会原样收下；Do 里还有一道兜底。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c, err := httpx.New(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Interceptors: httpx.Interceptors{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatalf("空链应回落默认链: %v", err)
	}
}

// ─────────────────────────────── options.go ───────────────────────────────

func TestRetryPolicy_归一化(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

	t.Run("零值取默认", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
		p := c.RetryPolicy()
		if p.MaxRetries != 3 || p.BaseBackoff != 200*time.Millisecond || p.MaxBackoff != 0 {
			t.Fatalf("p = %+v", p)
		}
		if p.IsRetryable == nil {
			t.Fatal("IsRetryable 应被补上默认实现")
		}
	})

	t.Run("NoRetry 显式关闭且不被 env 覆盖", func(t *testing.T) {
		t.Setenv("HTTPKIT_HTTP_MAX_RETRIES", "9")
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}, Retry: httpx.NoRetry()})
		if got := c.RetryPolicy().MaxRetries; got != 0 {
			t.Fatalf("MaxRetries = %d, want 0(显式设置优先于 env)", got)
		}
	})

	t.Run("env 覆盖默认", func(t *testing.T) {
		t.Setenv("HTTPKIT_HTTP_MAX_RETRIES", "1")
		t.Setenv("HTTPKIT_HTTP_RETRY_BACKOFF_MS", "5")
		t.Setenv("HTTPKIT_HTTP_RETRY_MAX_BACKOFF_MS", "7")
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
		p := c.RetryPolicy()
		if p.MaxRetries != 1 || p.BaseBackoff != 5*time.Millisecond || p.MaxBackoff != 7*time.Millisecond {
			t.Fatalf("p = %+v", p)
		}
	})

	t.Run("负重试次数与非法退避被纠正", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: srv.URL},
			Retry:   &httpx.RetryPolicy{MaxRetries: -5, BaseBackoff: -1, IsRetryable: func(error) bool { return true }},
		})
		p := c.RetryPolicy()
		if p.MaxRetries != 0 {
			t.Fatalf("负重试次数应纠正为 0, got %d", p.MaxRetries)
		}
		if p.BaseBackoff != 200*time.Millisecond {
			t.Fatalf("非法退避应回落默认, got %v", p.BaseBackoff)
		}
	})

	t.Run("WithRetry 全字段生效", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: srv.URL},
			Retry:   httpx.WithRetry(2, 3*time.Millisecond, 4*time.Millisecond),
		})
		p := c.RetryPolicy()
		if p.MaxRetries != 2 || p.BaseBackoff != 3*time.Millisecond || p.MaxBackoff != 4*time.Millisecond {
			t.Fatalf("p = %+v", p)
		}
	})
}

func TestOptions_超时默认值与env(t *testing.T) {
	t.Run("默认 30s", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
		if c.HTTPClient.Timeout != 30*time.Second {
			t.Fatalf("timeout = %v", c.HTTPClient.Timeout)
		}
	})
	t.Run("env 覆盖", func(t *testing.T) {
		t.Setenv("HTTPKIT_HTTP_TIMEOUT_MS", "1500")
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
		if c.HTTPClient.Timeout != 1500*time.Millisecond {
			t.Fatalf("timeout = %v", c.HTTPClient.Timeout)
		}
	})
	t.Run("显式值优先于 env", func(t *testing.T) {
		t.Setenv("HTTPKIT_HTTP_TIMEOUT_MS", "1500")
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}, Timeout: 9 * time.Second})
		if c.HTTPClient.Timeout != 9*time.Second {
			t.Fatalf("timeout = %v", c.HTTPClient.Timeout)
		}
	})
}

func TestOptions_自带Transport则不再调优也不接代理(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 999}
	c := newClientWith(t, httpx.Options{
		Headers:   httpx.StaticHeaders{Base: "https://x.example"},
		Transport: custom,
		ProxyURL:  "ftp://bad:1", // 非法代理也不该报错——因为根本不会去接
	})
	if c.HTTPClient.Transport != custom {
		t.Fatal("应原样使用调用方的 Transport")
	}
}

func TestOptions_DisableOriginReferer(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.DisableOriginReferer = true })
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	h := srv.last().Header
	if h.Get("origin") != "" || h.Get("referer") != "" {
		t.Fatalf("关闭后不该注入 origin/referer: %v", h)
	}
}

// ─────────────────────────────── transport.go ───────────────────────────────

func TestNewTransport_调优参数与代理错误(t *testing.T) {
	tr, err := httpx.NewTransport("")
	if err != nil {
		t.Fatal(err)
	}
	if !tr.DisableCompression {
		t.Fatal("必须关掉标准库自动 gzip，否则与自己的解压层重复")
	}
	if !tr.ForceAttemptHTTP2 || tr.TLSClientConfig == nil || tr.ResponseHeaderTimeout == 0 {
		t.Fatalf("调优参数缺失: %+v", tr)
	}
	if _, err := httpx.NewTransport("ftp://x:1"); err == nil {
		t.Fatal("非法代理应报错")
	}
}

func TestNewTransport_响应头超时env非法时回落(t *testing.T) {
	// 设成 0 等于关闭该保护，属于危险的静默降级，必须被纠正回默认值。
	t.Setenv("HTTPKIT_HTTP_RESPONSE_HEADER_TIMEOUT_MS", "0")
	tr, err := httpx.NewTransport("")
	if err != nil {
		t.Fatal(err)
	}
	if tr.ResponseHeaderTimeout != 15*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 回落 15s", tr.ResponseHeaderTimeout)
	}
}

// ─────────────────────────────── presets.go ───────────────────────────────

func TestChainEditors(t *testing.T) {
	a, b, term := mark("a"), mark("b"), mark("t")
	base := httpx.Interceptors{a, term}

	t.Run("Prepend", func(t *testing.T) {
		got := httpx.Prepend(base, b)
		if len(got) != 3 || got[0] != b {
			t.Fatalf("got %v", got)
		}
		if len(base) != 2 {
			t.Fatal("不该改动原链")
		}
	})

	t.Run("SpliceBeforeTerminal", func(t *testing.T) {
		got := httpx.SpliceBeforeTerminal(base, b)
		if len(got) != 3 || got[1] != b || got[2] != term {
			t.Fatalf("got %v", got)
		}
		if same := httpx.SpliceBeforeTerminal(base); len(same) != 2 {
			t.Fatal("不传拦截器应原样返回")
		}
		if got := httpx.SpliceBeforeTerminal(httpx.Interceptors{}, b); len(got) != 1 || got[0] != b {
			t.Fatalf("空链应直接返回 items, got %v", got)
		}
	})

	isA := func(it httpx.Interceptor) bool { return it == a }
	never := func(httpx.Interceptor) bool { return false }

	t.Run("InsertBefore", func(t *testing.T) {
		got := httpx.InsertBefore(base, isA, b)
		if len(got) != 3 || got[0] != b || got[1] != a {
			t.Fatalf("got %v", got)
		}
		if got := httpx.InsertBefore(base, never, b); len(got) != 2 {
			t.Fatal("无匹配应原样返回")
		}
		if got := httpx.InsertBefore(base, isA); len(got) != 2 {
			t.Fatal("不传拦截器应原样返回")
		}
	})

	t.Run("InsertAfter", func(t *testing.T) {
		got := httpx.InsertAfter(base, isA, b)
		if len(got) != 3 || got[0] != a || got[1] != b {
			t.Fatalf("got %v", got)
		}
		if got := httpx.InsertAfter(base, never, b); len(got) != 2 {
			t.Fatal("无匹配应原样返回")
		}
		if got := httpx.InsertAfter(base, isA); len(got) != 2 {
			t.Fatal("不传拦截器应原样返回")
		}
	})

	t.Run("Replace", func(t *testing.T) {
		got := httpx.Replace(base, isA, b)
		if got[0] != b {
			t.Fatalf("got %v", got)
		}
		if base[0] != a {
			t.Fatal("不该改动原链")
		}
		if got := httpx.Replace(base, never, b); got[0] != a {
			t.Fatal("无匹配应原样返回")
		}
	})

	t.Run("Without", func(t *testing.T) {
		got := httpx.Without(base, isA)
		if len(got) != 1 || got[0] != term {
			t.Fatalf("got %v", got)
		}
		if got := httpx.Without(base, never); len(got) != 2 {
			t.Fatal("无匹配应保留全部")
		}
	})

	t.Run("IsType 按自定义类型匹配", func(t *testing.T) {
		obs := &customObserver{}
		chain := httpx.Interceptors{a, obs, term}
		got := httpx.Without(chain, httpx.IsType[*customObserver]())
		if len(got) != 2 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("IsTerminal 识别两种终端", func(t *testing.T) {
		if !httpx.IsTerminal(httpx.DefaultChain()[len(httpx.DefaultChain())-1]) {
			t.Fatal("callServer 应被识别为终端")
		}
		if !httpx.IsTerminal(httpx.NoRedirectChain()[len(httpx.NoRedirectChain())-1]) {
			t.Fatal("noRedirectCallServer 应被识别为终端")
		}
		if httpx.IsTerminal(a) {
			t.Fatal("普通拦截器不是终端")
		}
	})

	t.Run("IsSideChannel", func(t *testing.T) {
		if !httpx.IsSideChannel(httpx.NewTransactionInterceptor(nil)) {
			t.Fatal("Transaction 拦截器应是旁路层")
		}
		if !httpx.IsSideChannel(httpx.NewHTMLSaveInterceptor(nil)) {
			t.Fatal("HTMLSave 拦截器应是旁路层")
		}
		if httpx.IsSideChannel(a) {
			t.Fatal("普通拦截器不是旁路层")
		}
	})
}

// markInterceptor 是可比较的占位拦截器（InterceptorFunc 是函数类型，不可用 == 比较）。
type markInterceptor struct{ name string }

func (m *markInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) { return ch.Proceed() }

func mark(name string) httpx.Interceptor { return &markInterceptor{name: name} }

func TestAPIChain_classify为nil时只加状态语义层(t *testing.T) {
	withClassify := httpx.APIChain(func(int, []byte) error { return nil })
	without := httpx.APIChain(nil)
	if len(withClassify) != len(without)+1 {
		t.Fatalf("len = %d / %d", len(withClassify), len(without))
	}
}

// ─────────────────────────── 可选拦截器的剩余分支 ───────────────────────────

func TestHTMLSaveInterceptor(t *testing.T) {
	t.Run("只保存 text/html", func(t *testing.T) {
		var saved [][]byte
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/html" {
				w.Header().Set("content-type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte("<p>hi</p>"))
				return
			}
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"a":1}`))
		})
		c := newClient(t, srv.Server, func(o *httpx.Options) {
			o.Interceptors = httpx.Prepend(httpx.DefaultChain(),
				httpx.NewHTMLSaveInterceptor(func(b []byte) { saved = append(saved, b) }))
		})
		if _, err := c.Get(context.Background(), "/html", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(context.Background(), "/json", nil); err != nil {
			t.Fatal(err)
		}
		if len(saved) != 1 || string(saved[0]) != "<p>hi</p>" {
			t.Fatalf("saved = %v", saved)
		}
	})

	t.Run("sink 为 nil 不炸", func(t *testing.T) {
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/html")
			_, _ = w.Write([]byte("<p>hi</p>"))
		})
		c := newClient(t, srv.Server, func(o *httpx.Options) {
			o.Interceptors = httpx.Prepend(httpx.DefaultChain(), httpx.NewHTMLSaveInterceptor(nil))
		})
		if _, err := c.Get(context.Background(), "/x", nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("出错时不保存", func(t *testing.T) {
		var saved int
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: "https://x.example"},
			Interceptors: httpx.Interceptors{
				httpx.NewHTMLSaveInterceptor(func([]byte) { saved++ }),
				httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
					return nil, errors.New("boom")
				}),
			},
		})
		if _, err := c.Get(context.Background(), "/x", nil); err == nil {
			t.Fatal("want error")
		}
		if saved != 0 {
			t.Fatal("出错不该保存")
		}
	})
}

func TestTransactionInterceptor_sink为nil与出错不回调(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(httpx.DefaultChain(), httpx.NewTransactionInterceptor(nil))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}

	var called int
	c2 := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			httpx.NewTransactionInterceptor(func(*httpx.Transaction) { called++ }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, errors.New("boom") }),
		},
	})
	if _, err := c2.Get(context.Background(), "/x", nil); err == nil {
		t.Fatal("want error")
	}
	if called != 0 {
		t.Fatal("resp 为 nil 时不该回调")
	}
}

func TestClassifyInterceptor_classify为nil时放行(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(httpx.DefaultChain(), httpx.NewClassifyInterceptor(nil))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyInterceptor_内层出错时不覆盖结论(t *testing.T) {
	sentinel := errors.New("inner")
	var classified int
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			httpx.NewClassifyInterceptor(func(int, []byte) error { classified++; return errors.New("outer") }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(context.Background(), "/x", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 内层错误原样穿透", err)
	}
	if classified != 0 {
		t.Fatal("内层已判失败时不该再跑归类")
	}
}

func TestStatusSemantics_2xx直接放行(t *testing.T) {
	var ruleCalls int
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(httpx.DefaultChain(),
			httpx.NewStatusSemanticsInterceptor(func(int, []byte) error { ruleCalls++; return errors.New("x") }))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	if ruleCalls != 0 {
		t.Fatal("2xx 不该进规则")
	}
}

func TestStatusSemantics_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			httpx.NewStatusSemanticsInterceptor(nil),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	if _, err := c.Get(context.Background(), "/x", nil); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestRetryableTextRule_未命中时回落默认规则(t *testing.T) {
	rule := httpx.RetryableTextRule([]string{"slow down"}, 572)
	if err := rule(500, []byte("something else")); err != nil {
		t.Fatalf("未命中关键词且有响应体时应放行, got %v", err)
	}
	if err := rule(500, nil); err == nil {
		t.Fatal("空体应报 HTTPStatusError")
	}
	if err := rule(572, []byte("x")); err == nil {
		t.Fatal("命中状态码应报 RetryableError")
	}
	if err := rule(400, []byte("please slow down now")); err == nil {
		t.Fatal("命中关键词应报 RetryableError")
	}
	if err := rule(400, []byte("normal body")); err != nil {
		t.Fatalf("未命中且有体应放行, got %v", err)
	}
}

func TestRetryableTextRule_空关键词被忽略(t *testing.T) {
	rule := httpx.RetryableTextRule([]string{""})
	if err := rule(400, []byte("anything")); err != nil {
		t.Fatalf("空关键词不该命中一切, got %v", err)
	}
}

func TestHTMLText_非HTML响应不动(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"html":"<p>x</p>"}`))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(httpx.DefaultChain(), httpx.NewHTMLTextInterceptor("<p>"))
	})
	body, err := c.Get(context.Background(), "/x", nil)
	if err != nil {
		t.Fatalf("非 HTML 响应不该被错误页标记误伤: %v", err)
	}
	if string(body) != `{"html":"<p>x</p>"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestHTMLText_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			httpx.NewHTMLTextInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	if _, err := c.Get(context.Background(), "/x", nil); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestMutator_mutate为nil不炸(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(httpx.DefaultChain(), httpx.NewRequestMutatorInterceptor(nil))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
}

// ─────────────────────────── 观察层与核心层的剩余分支 ───────────────────────────

func TestLogging_慢请求降级为warn(t *testing.T) {
	t.Setenv("HTTPKIT_HTTP_SLOW_MS", "1")
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	})
	// 全量与摘要两种日志形态都走一遍 slow 分支
	for _, summary := range []bool{false, true} {
		c := newClient(t, srv.Server, func(o *httpx.Options) { o.LogSummaryOnly = summary })
		if _, err := c.Get(context.Background(), "/x", nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLogging_摘要模式下4xx仍打全量(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("nope"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.LogSummaryOnly = true })
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	if c.SnapshotResponseStatusCode() != 404 {
		t.Fatal("状态码没记上")
	}
}

func TestLogging_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			httpx.NewLoggingInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	if _, err := c.Get(context.Background(), "/x", nil); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusCodeCache与响应头缓存_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	for name, chain := range map[string]httpx.Interceptors{
		"statusCodeCache":     {httpx.NewStatusCodeCacheInterceptor(), failing(sentinel)},
		"responseHeaderCache": {httpx.NewResponseHeaderCacheInterceptor(), failing(sentinel)},
		"bodyDecode":          {httpx.NewBodyDecodeInterceptor(), failing(sentinel)},
	} {
		t.Run(name, func(t *testing.T) {
			c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}, Interceptors: chain})
			if _, err := c.Get(context.Background(), "/x", nil); !errors.Is(err, sentinel) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func failing(err error) httpx.Interceptor {
	return httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, err })
}

func TestBodyDecode_Raw为nil时原样放过(t *testing.T) {
	// 回放层 / 自定义终端可能已经把 Body 填好了，此时解压层无事可做。
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			httpx.NewBodyDecodeInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				return &httpx.Response{StatusCode: 200, Header: http.Header{}, Body: []byte("replayed")}, nil
			}),
		},
	})
	body, err := c.Get(context.Background(), "/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "replayed" {
		t.Fatalf("body = %q", body)
	}
}

func TestBodyDecode_压缩流损坏时报错(t *testing.T) {
	for _, enc := range []string{"gzip", "zstd"} {
		t.Run(enc, func(t *testing.T) {
			srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("content-encoding", enc)
				_, _ = w.Write([]byte("this is not compressed at all"))
			})
			c := newClient(t, srv.Server, nil)
			if _, err := c.Get(context.Background(), "/x", nil); err == nil {
				t.Fatalf("%s 流损坏应报错", enc)
			}
		})
	}
}

func TestBridge_创建请求失败(t *testing.T) {
	// 非法方法名会让 http.NewRequestWithContext 直接失败，且不该被重试。
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(context.Background(), httpx.RequestSpec{Method: "BAD METHOD", Path: "/x"})
	if err == nil || !strings.Contains(err.Error(), "failed to create request") {
		t.Fatalf("err = %v", err)
	}
	if srv.count() != 0 {
		t.Fatal("请求都没建出来，不该发出去")
	}
}

func TestSnapshotRequestHeaders_nil与非首次尝试时不动(t *testing.T) {
	httpx.SnapshotRequestHeaders(nil) // 不该 panic
	req := &httpx.Request{Attempt: 1}
	httpx.SnapshotRequestHeaders(req)
	if req.ReqHeaders != nil {
		t.Fatal("非首次尝试不该覆盖快照")
	}
	req2 := &httpx.Request{Attempt: 0}
	httpx.SnapshotRequestHeaders(req2)
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
	if got := req.ReqHeaders["host"]; len(got) != 1 || got[0] != "fallback.example" {
		t.Fatalf("host = %v, want 从 URL 回落", got)
	}
}

// newClientWith 用给定 Options 建客户端（失败即 fatal）。
func newClientWith(t *testing.T, opts httpx.Options) *httpx.Client {
	t.Helper()
	c, err := httpx.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

var _ = httptest.NewServer // 保持 httptest 导入（部分用例经 newRecordingServer 间接使用）

func TestNoRedirectTerminal_传输错误也包成TransportError(t *testing.T) {
	// 禁重定向终端和默认终端必须对错误做同样的包装，否则那条链上的重试会失效。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	var attempts int
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Retry:        httpx.WithRetry(2, time.Millisecond, 0),
		Interceptors: httpx.SpliceBeforeTerminal(httpx.NoRedirectChain(), countingInterceptor(&attempts)),
	})
	if _, err := c.Get(context.Background(), "/x", nil); err == nil {
		t.Fatal("want error")
	}
	if attempts != 3 {
		t.Fatalf("尝试 %d 次，want 3(禁重定向终端的错误同样应触发重试)", attempts)
	}
}

func countingInterceptor(n *int) httpx.Interceptor {
	return httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		*n++
		return ch.Proceed()
	})
}
