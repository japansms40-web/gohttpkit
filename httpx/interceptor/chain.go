// Package interceptor 汇集 httpx 的内建拦截器与预设链。
//
// httpx 只保留链框架（Interceptor / Chain / Request / Response / Client）；所有具体拦截器
// （重试、桥接、终端、解压、日志、缓存、状态语义、归类、HTML 处理）以及 DefaultChain /
// NoRedirectChain / APIChain 的装配都在本包，单向依赖 httpx，避免父子包循环引用。
// 一拦截器一文件；开箱即用入口是 httpx.NewClient（import 本包即注册默认链）。
package interceptor

import "github.com/japansms40-web/gohttpkit/httpx"

// DefaultChain 默认链（外 → 内）：
//
//	tracing → logging → bodyDecode → statusCodeCache → responseHeaderCache → retry → bridge → callServer
//
// 给普通 HTTP 调用：不做业务解释，只做重试、解压、日志、状态缓存。
// 输入：无。返回新切片，每次调用独立分配。
// 返回：8 层预设；非 2xx 不报错；解码失败不重试。
// 例：httpx.NewClient(httpx.Options{Headers: h}) 未传 Interceptors 时用本链。
func DefaultChain() httpx.Interceptors {
	return baseChain(NewCallServerInterceptor())
}

// NoRedirectChain 禁重定向链。
// 给登录/授权：要靠 302 Location / Set-Cookie 判断状态机，跟随重定向会吃掉信息。
// 输入：无。返回新切片，骨架与 DefaultChain 相同，只换终端。
// 返回：3xx 原样交给调用方。例：client.WithChain(interceptor.NoRedirectChain())。
func NoRedirectChain() httpx.Interceptors {
	return baseChain(NewNoRedirectCallServerInterceptor())
}

func baseChain(terminal httpx.Interceptor) httpx.Interceptors {
	return httpx.Interceptors{
		NewTracingInterceptor(),
		NewLoggingInterceptor(),
		NewBodyDecodeInterceptor(),
		NewStatusCodeCacheInterceptor(),
		NewResponseHeaderCacheInterceptor(),
		NewRetryInterceptor(),
		NewBridgeInterceptor(),
		terminal,
	}
}

// APIChain 面向 JSON API 的常用链。
// 给 REST/JSON 接入：在默认链外层加上「非 2xx 空体 → error」的状态语义层。
// 输入 classify：可选业务错误归类；nil 时不加 classify 层。
// 返回：新切片 = [classify?] + statusSemantics + DefaultChain()。
func APIChain(classify func(status int, body []byte) error) httpx.Interceptors {
	chain := httpx.Interceptors{}
	if classify != nil {
		chain = append(chain, NewClassifyInterceptor(classify))
	}
	chain = append(chain, NewStatusSemanticsInterceptor(nil))
	return append(chain, DefaultChain()...)
}
