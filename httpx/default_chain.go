package httpx

import "sync/atomic"

// default_chain.go —— 默认拦截器链的注册点与开箱即用的 NewClient。
//
// 默认链（DefaultChain）实现在 httpx/interceptor，而 interceptor 依赖 httpx，httpx 不能反向 import 它。
// 所以由 interceptor 包在 init() 里调 RegisterDefaultChain 把工厂注册进来（同 database/sql 驱动模式）。
//
// 并发模型：defaultChain 是包级共享的工厂指针；写只发生在包初始化期（RegisterDefaultChain），
// 读发生在任意 goroutine 的 NewClient。用 atomic.Pointer 保证读写无竞态，不加锁、不在临界区做 IO。

// defaultChain 保存注册的默认链工厂；nil 表示尚未注册。
var defaultChain atomic.Pointer[func() Interceptors]

// RegisterDefaultChain 注册 NewClient 在未传 Interceptors 时使用的默认链工厂。
// 给 httpx/interceptor 包的 init() 用；业务代码一般不直接调用。
// 输入 f：每次 NewClient 调一次，必须返回新切片（链不跨 Client 共享）；f 为 nil 时忽略，不清掉已注册的工厂。
// 返回：无。后注册的覆盖先注册的。
func RegisterDefaultChain(f func() Interceptors) {
	if f == nil {
		return
	}
	defaultChain.Store(&f)
}

// NewClient 创建带默认链的客户端，是开箱即用的入口。
// 给接入方：只 import httpx 调本函数即可，默认链由 httpx/interceptor 注册——
// 程序里至少要有一处 import 该包（用到任何拦截器即已满足；否则加 `import _ ".../httpx/interceptor"`）。
// 输入 opts：规则同 New；Interceptors 为 nil 时填注册的默认链，空切片（非 nil）视为调用方显式自组链，不回落默认链。
// 返回：同 New；Headers==nil 是 *MissingHeaderProviderError；Interceptors 为 nil 且未注册默认链是 *NoDefaultChainError。
// 例：NewClient(Options{Headers: StaticHeaders{Base: "https://api.example.com"}}) → 带默认链的 *Client。
func NewClient(opts Options) (*Client, error) {
	if opts.Headers == nil {
		return nil, &MissingHeaderProviderError{Field: fieldOptionsHeaders}
	}
	if opts.Interceptors == nil {
		f := defaultChain.Load()
		if f == nil {
			return nil, &NoDefaultChainError{}
		}
		opts.Interceptors = (*f)()
	}
	return New(opts)
}
