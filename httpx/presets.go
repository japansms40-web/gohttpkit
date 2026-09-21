package httpx

// presets.go —— 链编辑工具。
//
// 预设链（DefaultChain / NoRedirectChain / APIChain）在 httpx/interceptor。
// 本文件只提供对任意链切片的编辑：全部返回新切片，不改原链。

// ─────────────────────────────── 链编辑工具 ───────────────────────────────
//
// 全部返回新切片，不改原链 —— 链是共享的（多个 Client 可能持有同一条），就地改会串扰。

// Prepend 把拦截器插到链最外层（首位）。
// 给加业务层（classify / 状态语义 / 埋点）：它们要看最终响应体。
// 输入 chain：原链，不改写；items 为空仍分配新切片（原链的浅拷贝）。
// 返回：始终是新切片，items 在前、chain 在后。
// 例：Prepend(interceptor.DefaultChain(), interceptor.NewClassifyInterceptor(fn))。
func Prepend(chain Interceptors, items ...Interceptor) Interceptors {
	return append(append(Interceptors{}, items...), chain...)
}

// SpliceBeforeTerminal 把拦截器插到链终端（最后一个）之前。
// 给计费抑制、发送计数、限速器：位于 retry 之内、网络发送之外，按真实出网次数生效。
// 输入 chain / items：都不改写。
// 返回：items 为空 → 原切片（不分配）；chain 为空 → 新切片=items；否则新切片。
// 例：SpliceBeforeTerminal(interceptor.DefaultChain(), interceptor.NewRequestMutatorInterceptor(fn))。
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

// InsertBefore 把 items 插到第一个满足 match 的拦截器之前。
// 给按语义锚点插层：例如插到终端或旁路观察层之前。
// 输入 chain / match / items：不改 chain；match 为 nil 会 panic。
// 返回：无匹配或 items 为空 → 原切片；否则新切片。
// 例：InsertBefore(chain, IsTerminal, extra)；match 没命中 → 同一份 chain。
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

// InsertAfter 把 items 插到第一个满足 match 的拦截器之后。
// 给按语义锚点插层。输入 / 返回规则同 InsertBefore。
// 例：InsertAfter(chain, IsSideChannel, extra)；match 没命中 → 同一份 chain。
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

// Replace 把第一个满足 match 的拦截器换成 replacement。
// 给换终端：把 callServer 换成禁重定向变体或回放层。
// 输入 chain / match / replacement：不改 chain；match 为 nil 会 panic。
// 返回：无匹配 → 原切片；命中 → 新切片（浅拷贝后改那一格）。
// 例：Replace(interceptor.DefaultChain(), IsTerminal, interceptor.NewNoRedirectCallServerInterceptor())。
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
// 给去掉默认层（如关掉 logging）的调用方。
// 输入 chain / match：不改 chain；match 为 nil 会 panic。
// 返回：始终新切片（可能更短，也可能等长）；一个都没删也是新分配。
// 例：Without(DefaultChain(), IsSideChannel)。
func Without(chain Interceptors, match func(Interceptor) bool) Interceptors {
	out := make(Interceptors, 0, len(chain))
	for _, it := range chain {
		if !match(it) {
			out = append(out, it)
		}
	}
	return out
}

// indexOf 找第一个满足 match 的下标。
// 给 InsertBefore / InsertAfter / Replace。
// 输入 chain：可为 nil/空；match 为 nil 会 panic。不改 chain。
// 返回：命中是 [0, len)；未命中是 -1。
func indexOf(chain Interceptors, match func(Interceptor) bool) int {
	for i, it := range chain {
		if match(it) {
			return i
		}
	}
	return -1
}

// IsType 生成一个按具体类型匹配的谓词。
// 给匹配【你自己定义】的拦截器类型；本库内建类型非导出，请用 IsTerminal / IsSideChannel。
// 输入：类型参数 T。返回：谓词闭包，it 断言成 T 则 true。
// 例：Replace(chain, IsType[*mySigner](), newSigner)。
func IsType[T any]() func(Interceptor) bool {
	return func(it Interceptor) bool {
		_, ok := it.(T)
		return ok
	}
}

// IsTerminal 匹配终端拦截器。
// 给 Replace / InsertBefore：定位唯一接触网络的那一层。
// 输入 it：可为 nil，此时 false。
// 返回：实现 Terminal 为 true。例：Replace(chain, IsTerminal, replay)。
func IsTerminal(it Interceptor) bool {
	_, ok := it.(Terminal)
	return ok
}

// IsSideChannel 匹配旁路观察拦截器。
// 给链编辑与诊断：识别内嵌 SideChannelMarker 的层。
// 输入 it：可为 nil，此时 false。
// 返回：实现 SideChannel 为 true。例：Without(chain, IsSideChannel)。
func IsSideChannel(it Interceptor) bool {
	_, ok := it.(SideChannel)
	return ok
}
