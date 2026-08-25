package httpx

// presets.go —— 预设链与链编辑工具。
//
// 本库的默认链【不做任何业务解释】：不提纯 HTML、不归类业务错误、不把非 2xx 变成 error。
// 它只把「所有 HTTP 客户端都要做的脏活」做好：重试、解压、日志、状态缓存。
// 业务语义一律以可选拦截器的形式由你显式加进来 —— 加什么、加在哪一层，你说了算。

// DefaultChain 默认链（外 → 内）：
//
//	logging → bodyDecode → statusCodeCache → responseHeaderCache → retry → bridge → callServer
//
// 语义：
//   - 非 2xx 不报错，响应体原样返回，状态码经 Client.SnapshotResponseStatusCode() 读
//   - 响应体按 content-encoding 解压后返回原始字节，不做任何内容改写
//   - 网络错误自动重试（默认 3 次 + 200/400/800ms 指数退避）
//   - 响应头缓存 + 会话状态回写（Options.OnResponseHeaders）
//
// 状态码与响应头缓存都放在 retry 之外（重试跑完才记）、bodyDecode 之内
// （解压失败也已经记下），两者时机一致，不留「状态码记了但响应头没记」这种边角。
//
// 你自己的业务层（错误归类、状态语义、埋点）用 Prepend 加在【更外层】，
// 这样它们看到的是最终响应体，而内层的状态回写仍然照常发生 —— 哪怕你的业务层判定失败。
func DefaultChain() Interceptors {
	return Interceptors{
		NewLoggingInterceptor(),
		NewBodyDecodeInterceptor(),
		NewStatusCodeCacheInterceptor(),
		NewResponseHeaderCacheInterceptor(),
		NewRetryInterceptor(),
		NewBridgeInterceptor(),
		NewCallServerInterceptor(),
	}
}

// NoRedirectChain 禁重定向链：与 DefaultChain 相同，只把终端换成禁重定向变体，
// 把 3xx 原样交给调用方（读 Location、读 302 上的 Set-Cookie）。
//
// 典型用途：登录/授权类流程要靠 302 的 Location 判断状态机走到了哪一步，
// 跟随重定向会把这个信息吃掉。
func NoRedirectChain() Interceptors {
	return Interceptors{
		NewLoggingInterceptor(),
		NewBodyDecodeInterceptor(),
		NewStatusCodeCacheInterceptor(),
		NewResponseHeaderCacheInterceptor(),
		NewRetryInterceptor(),
		NewBridgeInterceptor(),
		NewNoRedirectCallServerInterceptor(),
	}
}

// APIChain 面向 JSON API 的常用链：在默认链外层加上「非 2xx 空体 → error」的状态语义层，
// 以及可选的业务错误归类层（classify 为 nil 时不加）。
//
// classify 拿到的是最终响应体，返回非 nil error 即让整次调用以该 error 失败 ——
// 用来把服务端那套 {"status":"fail","message":"..."} 统一翻译成你的 sentinel error，
// 免得每个业务函数都手写一遍判断。
func APIChain(classify func(status int, body []byte) error) Interceptors {
	chain := Interceptors{}
	if classify != nil {
		chain = append(chain, NewClassifyInterceptor(classify))
	}
	chain = append(chain, NewStatusSemanticsInterceptor(nil))
	return append(chain, DefaultChain()...)
}

// ─────────────────────────────── 链编辑工具 ───────────────────────────────
//
// 全部返回新切片，不改原链 —— 链是共享的（多个 Client 可能持有同一条），就地改会串扰。

// Prepend 把拦截器插到链最外层（首位）。
func Prepend(chain Interceptors, items ...Interceptor) Interceptors {
	return append(append(Interceptors{}, items...), chain...)
}

// SpliceBeforeTerminal 把拦截器插到链【终端（最后一个）之前】，即位于 retry 之内、
// 网络发送之外，包住每一次真实发送（含每次重试）。计费抑制、发送计数、限速器等
// 「按真实出网次数」生效的横切逻辑应该插在这里。
func SpliceBeforeTerminal(chain Interceptors, items ...Interceptor) Interceptors {
	if len(items) == 0 {
		return chain
	}
	if len(chain) == 0 {
		return append(Interceptors{}, items...)
	}
	n := len(chain)
	out := make(Interceptors, 0, n+len(items))
	out = append(out, chain[:n-1]...)
	out = append(out, items...)
	out = append(out, chain[n-1])
	return out
}

// InsertBefore 把 items 插到第一个满足 match 的拦截器之前；没有匹配则原样返回。
func InsertBefore(chain Interceptors, match func(Interceptor) bool, items ...Interceptor) Interceptors {
	idx := indexOf(chain, match)
	if idx < 0 || len(items) == 0 {
		return chain
	}
	out := make(Interceptors, 0, len(chain)+len(items))
	out = append(out, chain[:idx]...)
	out = append(out, items...)
	return append(out, chain[idx:]...)
}

// InsertAfter 把 items 插到第一个满足 match 的拦截器之后；没有匹配则原样返回。
func InsertAfter(chain Interceptors, match func(Interceptor) bool, items ...Interceptor) Interceptors {
	idx := indexOf(chain, match)
	if idx < 0 || len(items) == 0 {
		return chain
	}
	out := make(Interceptors, 0, len(chain)+len(items))
	out = append(out, chain[:idx+1]...)
	out = append(out, items...)
	return append(out, chain[idx+1:]...)
}

// Replace 把第一个满足 match 的拦截器换成 replacement；没有匹配则原样返回。
// 换终端（如把 callServer 换成禁重定向变体或回放层）就用它。
func Replace(chain Interceptors, match func(Interceptor) bool, replacement Interceptor) Interceptors {
	idx := indexOf(chain, match)
	if idx < 0 {
		return chain
	}
	out := append(Interceptors{}, chain...)
	out[idx] = replacement
	return out
}

// Without 移除所有满足 match 的拦截器。
func Without(chain Interceptors, match func(Interceptor) bool) Interceptors {
	out := make(Interceptors, 0, len(chain))
	for _, it := range chain {
		if !match(it) {
			out = append(out, it)
		}
	}
	return out
}

func indexOf(chain Interceptors, match func(Interceptor) bool) int {
	for i, it := range chain {
		if match(it) {
			return i
		}
	}
	return -1
}

// IsType 生成一个按具体类型匹配的谓词，用于匹配【你自己定义】的拦截器类型：
//
//	chain = httpx.Replace(chain, httpx.IsType[*mySigner](), newSigner)
//
// 本库内建拦截器的具体类型都是非导出的，匹配它们请用语义谓词 IsTerminal / IsSideChannel。
func IsType[T any]() func(Interceptor) bool {
	return func(it Interceptor) bool {
		_, ok := it.(T)
		return ok
	}
}

// IsTerminal 匹配终端拦截器（callServer / noRedirectCallServer）。
func IsTerminal(it Interceptor) bool {
	switch it.(type) {
	case *callServerInterceptor, *noRedirectCallServerInterceptor:
		return true
	default:
		return false
	}
}

// IsSideChannel 匹配旁路观察拦截器。
func IsSideChannel(it Interceptor) bool {
	_, ok := it.(SideChannel)
	return ok
}
