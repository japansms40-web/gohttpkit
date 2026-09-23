package httpx

import (
	"errors"
	"testing"
)

// noopInterceptor 是测试用的占位拦截器：只用来数链长，不会被执行。
var noopInterceptor = InterceptorFunc(func(*Chain) (*Response, error) { return nil, nil })

// swapDefaultChain 临时替换注册的默认链工厂，测试结束还原。改的是包级状态，调用方不得 t.Parallel()。
func swapDefaultChain(t *testing.T, f func() Interceptors) {
	t.Helper()
	old := defaultChain.Load()
	if f == nil {
		defaultChain.Store(nil)
	} else {
		defaultChain.Store(&f)
	}
	t.Cleanup(func() { defaultChain.Store(old) })
}

func TestNewClient_未注册默认链返回类型错误(t *testing.T) {
	swapDefaultChain(t, nil)
	c, err := NewClient(Options{Headers: StaticHeaders{Base: "https://x.example"}})
	t.Logf("c=%v err=%v", c, err)
	var ne *NoDefaultChainError
	if !errors.As(err, &ne) || c != nil {
		t.Fatalf("err = %v (%T), want *NoDefaultChainError 且 client 为 nil", err, err)
	}
}

func TestNewClient_Headers缺失优先报错(t *testing.T) {
	swapDefaultChain(t, nil)
	_, err := NewClient(Options{})
	var me *MissingHeaderProviderError
	if !errors.As(err, &me) || me.Field != "Options.Headers" {
		t.Fatalf("err = %v (%T), want *MissingHeaderProviderError{Field:Options.Headers}", err, err)
	}
}

func TestNewClient_每个Client各调一次工厂(t *testing.T) {
	calls := 0
	swapDefaultChain(t, func() Interceptors { calls++; return Interceptors{noopInterceptor} })
	for i := 0; i < 2; i++ {
		c, err := NewClient(Options{Headers: StaticHeaders{Base: "https://x.example"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Interceptors()) != 1 {
			t.Fatalf("链长 = %d, want 1", len(c.Interceptors()))
		}
	}
	t.Logf("工厂调用次数=%d", calls)
	if calls != 2 {
		t.Fatalf("工厂应每个 Client 调一次（链不跨 Client 共享），got %d", calls)
	}
}

func TestNewClient_显式链不调工厂(t *testing.T) {
	calls := 0
	swapDefaultChain(t, func() Interceptors { calls++; return nil })
	for _, chain := range []Interceptors{{}, {noopInterceptor}} {
		c, err := NewClient(Options{Headers: StaticHeaders{Base: "https://x.example"}, Interceptors: chain})
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Interceptors()) != len(chain) {
			t.Fatalf("显式链被改写：len=%d want %d", len(c.Interceptors()), len(chain))
		}
	}
	if calls != 0 {
		t.Fatalf("显式传链（含空切片）不应调默认链工厂，got %d 次", calls)
	}
}

func TestRegisterDefaultChain_nil忽略(t *testing.T) {
	sentinel := func() Interceptors { return Interceptors{noopInterceptor} }
	swapDefaultChain(t, sentinel)
	RegisterDefaultChain(nil)
	if defaultChain.Load() == nil {
		t.Fatal("RegisterDefaultChain(nil) 不应清掉已注册的工厂")
	}
}

func TestNoDefaultChainError_文案(t *testing.T) {
	var nilErr *NoDefaultChainError
	for _, e := range []*NoDefaultChainError{nil, {}} {
		t.Logf("%v", e.Error())
	}
	if nilErr.Error() == (&NoDefaultChainError{}).Error() {
		t.Fatal("nil 接收者文案应与正常文案区分")
	}
}
