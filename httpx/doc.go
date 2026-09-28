// Package httpx 提供一套可直接复用的 HTTP 客户端基建：OkHttp 风格的拦截器链、
// 网络层重试与指数退避、四种压缩解码、代理接入、结构化日志、header 白名单精确发头、
// 请求/响应整包快照落盘。
//
// 它不绑定任何具体 API：请求头怎么构建由你实现 HeaderProvider 决定，业务错误怎么归类
// 由你写一层拦截器决定。库只负责把这些横切关注点组织成一条可预测、可测试、可扩展的链。
//
// 最小可用示例：
//
//	client, err := httpx.NewClient(httpx.Options{
//	    Headers: httpx.StaticHeaders{
//	        Base:    "https://api.example.com",
//	        Headers: map[string]string{"accept": "application/json"},
//	    },
//	})
//	body, err := client.Get(ctx, "/v1/ping", nil)
//
// NewClient 的默认链由 httpx/interceptor 包注册：程序里至少 import 一次该包（`import _ ".../httpx/interceptor"` 即可），
// 否则未传 Interceptors 时返回 *NoDefaultChainError。
//
// 更多用法见 examples/ 下三个可直接 go run 的例子。
//
// 文件结构：
//
//	httpx/
//	├── body.go           请求体编码、响应解码与日志 body 截断（TruncateBodyForLog）
//	├── chain.go          链框架：Interceptor / Chain / Request / Response 与 Terminal、SideChannel 标记
//	├── client.go         Client 与 Do / Get / PostForm / PostJSON 入口
//	├── default_chain.go  默认链注册点 RegisterDefaultChain 与 NewClient
//	├── doc.go            包文档（本文件）
//	├── encoding.go       ContentEncoding 枚举与 ParseContentEncoding
//	├── errors.go         本包类型错误与 Op 常量
//	├── events.go         HTTP 领域机器事件（http.transaction / http.retry 等）
//	├── header_names.go   本库用到的 header 名常量（全小写）
//	├── headers.go        HeaderProvider、StaticHeaders、白名单过滤与请求头快照
//	├── html.go           ExtractHTMLText：HTML 转纯文本
//	├── log_fields.go     HTTP 交易 / 重试日志字段 key，与 Transaction 的 json tag 同值
//	├── options.go        Options 与 RetryPolicy（NoRetry / WithRetry）
//	├── presets.go        链编辑工具：Prepend / SpliceBeforeTerminal / InsertBefore / Replace / Without 等
//	├── status_class.go   StatusClass 状态码分类
//	├── status_rule.go    StatusRule 状态语义规则：DefaultStatusRule / RetryableTextRule
//	├── transaction.go    Transaction：一次请求 / 响应的整包快照
//	├── transport.go      NewTransport：可接代理的 http.Transport
//	└── interceptor/      子包 interceptor：内建拦截器与预设链，import 即注册默认链
package httpx
