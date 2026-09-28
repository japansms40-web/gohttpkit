// Package interceptor 汇集 httpx 的内建拦截器与预设链。
//
// httpx 只保留链框架（Interceptor / Chain / Request / Response / Client）；所有具体拦截器
// （重试、桥接、终端、解压、日志、缓存、状态语义、归类、HTML 处理）以及 DefaultChain /
// NoRedirectChain / APIChain 的装配都在本包，单向依赖 httpx，避免父子包循环引用。
// 一拦截器一文件；开箱即用入口是 httpx.NewClient（import 本包即注册默认链）。
//
// 文件结构：
//
//	interceptor/
//	├── body_decode.go            按 content-encoding 解压响应体
//	├── bridge.go                 Request → *http.Request：拼 URL、白名单发头、编码 body
//	├── call_server.go            终端：跟随重定向发送
//	├── chain.go                  预设链装配：DefaultChain / NoRedirectChain / APIChain
//	├── classify.go               调用方提供的业务归类函数，把响应翻译成错误
//	├── do_http.go                两种终端共用的发送路径，网络错误包成 *httpx.TransportError
//	├── doc.go                    包文档（本文件）
//	├── errors.go                 本包 Op 常量
//	├── html_save.go              HTML 响应原文交给 sink 落盘
//	├── html_text.go              HTML 响应转纯文本，识别错误页标记
//	├── logging.go                HTTP 交易日志（event=http.transaction），含慢请求标记
//	├── no_redirect.go            终端：不跟随重定向发送
//	├── register.go               init 把 DefaultChain 注册为 httpx.NewClient 的默认链
//	├── request_mutator.go        发送前就地改写 *Request（删头、加签名、换出口）
//	├── response_header_cache.go  缓存最近一次响应头并回调 OnResponseHeaders
//	├── retry.go                  网络层重试与指数退避
//	├── status_code_cache.go      记录最近一次响应状态码
//	├── status_semantics.go       按 StatusRule 把状态码 / body 翻译成错误
//	├── tracing.go                给每次请求补 trace / span
//	└── transaction.go            旁路：整包请求 / 响应快照交给 sink
package interceptor
