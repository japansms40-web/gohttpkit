package httpx_test

// chain_test.go —— SideChannel、Chain 访问器与缺终端的单元契约。

import (
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

// ─────────────────────────────── chain.go ───────────────────────────────

func TestChain_缺终端拦截器给出明确错误(t *testing.T) {
	c, err := httpx.New(httpx.Options{
		Headers:      httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{interceptor.NewLoggingInterceptor()},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(t.Context(), "/x", nil)
	assertChainExhausted(t, err, 1, 1)
}

func TestTerminalMarker_可用于自定义终端(t *testing.T) {
	var it httpx.Interceptor = &customTerminal{}
	_, ok := it.(httpx.Terminal)
	t.Logf("Terminal=%v IsTerminal=%v nil=%v", ok, httpx.IsTerminal(it), httpx.IsTerminal(nil))
	if !ok {
		t.Fatal("内嵌 TerminalMarker 后应满足 Terminal 接口")
	}
	if !httpx.IsTerminal(it) {
		t.Fatal("IsTerminal 应识别自定义终端")
	}
	if httpx.IsTerminal(nil) {
		t.Fatal("nil 不是终端")
	}
	it.(httpx.Terminal).Terminal()
}

type customTerminal struct{ httpx.TerminalMarker }

func (customTerminal) Intercept(*httpx.Chain) (*httpx.Response, error) {
	return &httpx.Response{StatusCode: 200}, nil
}

func TestSideChannelMarker_可用于自定义旁路层(t *testing.T) {
	var it httpx.Interceptor = &customObserver{}
	_, ok := it.(httpx.SideChannel)
	t.Logf("SideChannel=%v IsSideChannel=%v", ok, httpx.IsSideChannel(it))
	if !ok {
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
		Interceptors: httpx.Prepend(interceptor.DefaultChain(), probe),
	})
	if err != nil {
		t.Fatal(err)
	}
	c = c2
	if _, err := c.Get(t.Context(), "/probe", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("path=%q sameClient=%v", gotPath, sameClient)
	if gotPath != "/probe" {
		t.Fatalf("Request().Path = %q", gotPath)
	}
	if !sameClient {
		t.Fatal("Client() 应返回发起请求的那个客户端")
	}
}
