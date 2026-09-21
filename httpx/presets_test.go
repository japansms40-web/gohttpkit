package httpx_test

// presets_test.go —— 预设链与链编辑工具的单元契约。

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

// ─────────────────────────────── presets.go ───────────────────────────────

func TestChainEditors(t *testing.T) {
	a, b, term := mark("a"), mark("b"), mark("t")
	base := httpx.Interceptors{a, term}

	t.Run("Prepend", func(t *testing.T) {
		got := httpx.Prepend(base, b)
		t.Logf("Prepend len=%d first=b baseLen=%d", len(got), len(base))
		if len(got) != 3 || got[0] != b {
			t.Fatalf("got %v", got)
		}
		if len(base) != 2 {
			t.Fatal("不该改动原链")
		}
	})

	t.Run("SpliceBeforeTerminal", func(t *testing.T) {
		got := httpx.SpliceBeforeTerminal(base, b)
		t.Logf("SpliceBeforeTerminal len=%d emptyItems=%d emptyChain=%d", len(got), len(httpx.SpliceBeforeTerminal(base)), len(httpx.SpliceBeforeTerminal(httpx.Interceptors{}, b)))
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
		t.Logf("InsertBefore len=%d miss=%d emptyItems=%d", len(got), len(httpx.InsertBefore(base, never, b)), len(httpx.InsertBefore(base, isA)))
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
		t.Logf("InsertAfter len=%d miss=%d emptyItems=%d", len(got), len(httpx.InsertAfter(base, never, b)), len(httpx.InsertAfter(base, isA)))
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
		t.Logf("Replace first=b missKeepsA=%v origFirstUnchanged=%v", httpx.Replace(base, never, b)[0] == a, base[0] == a)
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
		t.Logf("Without len=%d missLen=%d", len(got), len(httpx.Without(base, never)))
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
		t.Logf("IsType Without len=%d", len(got))
		if len(got) != 2 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("IsTerminal 识别两种终端", func(t *testing.T) {
		t.Logf("IsTerminal default=%v noRedirect=%v mark=%v", httpx.IsTerminal(interceptor.DefaultChain()[len(interceptor.DefaultChain())-1]), httpx.IsTerminal(interceptor.NoRedirectChain()[len(interceptor.NoRedirectChain())-1]), httpx.IsTerminal(a))
		if !httpx.IsTerminal(interceptor.DefaultChain()[len(interceptor.DefaultChain())-1]) {
			t.Fatal("callServer 应被识别为终端")
		}
		if !httpx.IsTerminal(interceptor.NoRedirectChain()[len(interceptor.NoRedirectChain())-1]) {
			t.Fatal("noRedirectCallServer 应被识别为终端")
		}
		if httpx.IsTerminal(a) {
			t.Fatal("普通拦截器不是终端")
		}
	})

	t.Run("IsSideChannel", func(t *testing.T) {
		t.Logf("IsSideChannel tx=%v html=%v mark=%v tracing=%v", httpx.IsSideChannel(interceptor.NewTransactionInterceptor(nil)), httpx.IsSideChannel(interceptor.NewHTMLSaveInterceptor(nil)), httpx.IsSideChannel(a), httpx.IsSideChannel(interceptor.NewTracingInterceptor()))
		if !httpx.IsSideChannel(interceptor.NewTransactionInterceptor(nil)) {
			t.Fatal("Transaction 拦截器应是旁路层")
		}
		if !httpx.IsSideChannel(interceptor.NewHTMLSaveInterceptor(nil)) {
			t.Fatal("HTMLSave 拦截器应是旁路层")
		}
		if httpx.IsSideChannel(a) {
			t.Fatal("普通拦截器不是旁路层")
		}
		if httpx.IsSideChannel(interceptor.NewTracingInterceptor()) {
			t.Fatal("tracing 会改写 Request.Ctx，不是旁路层")
		}
	})
}

func TestChain_默认链与禁重定向链含tracing(t *testing.T) {
	t.Logf("DefaultChain=%d NoRedirectChain=%d tracingSide=%v", len(interceptor.DefaultChain()), len(interceptor.NoRedirectChain()), httpx.IsSideChannel(interceptor.DefaultChain()[0]))
	if got := len(interceptor.DefaultChain()); got != 8 {
		t.Fatalf("DefaultChain 长度 = %d, want 8（最外层 tracing）", got)
	}
	if got := len(interceptor.NoRedirectChain()); got != 8 {
		t.Fatalf("NoRedirectChain 长度 = %d, want 8", got)
	}
	if httpx.IsSideChannel(interceptor.DefaultChain()[0]) || httpx.IsSideChannel(interceptor.NoRedirectChain()[0]) {
		t.Fatal("最外层 tracing 不应被标成旁路层")
	}
}

// markInterceptor 是可比较的占位拦截器（InterceptorFunc 是函数类型，不可用 == 比较）。
type markInterceptor struct{ name string }

func (m *markInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) { return ch.Proceed() }

func mark(name string) httpx.Interceptor { return &markInterceptor{name: name} }

func TestAPIChain_classify为nil时只加状态语义层(t *testing.T) {
	withClassify := interceptor.APIChain(func(int, []byte) error { return nil })
	without := interceptor.APIChain(nil)
	t.Logf("APIChain classify=%d nil=%d", len(withClassify), len(without))
	if len(withClassify) != len(without)+1 {
		t.Fatalf("len = %d / %d", len(withClassify), len(without))
	}
}
