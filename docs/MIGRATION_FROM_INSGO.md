# 从 insgo 迁移到 gohttpkit

本库的实现抽取自 insgo（`chenweilong1022/insgo`）的 HTTP 基建，去掉了全部 Instagram 业务耦合。
insgo 侧**未做任何改动**，两边是独立演进的两份代码；本文档记录符号对照与**行为差异**，
方便日后同步共性修复，也方便老项目照着改。

## 为什么要抽出来

1. insgo 的 HTTP 核心全在 `internal/`（`request.go` / `interceptor.go` / `interceptors.go` /
   `proxy.go` / `headers*.go`），Go 的 internal 规则决定了跨模块 import 不到；
2. insgo 的 `go.mod` 对 `chenweilong1022/insgouagen` 用了 `replace`，而 replace 对下游消费者无效，
   别的项目 `go get chenweilong1022/insgo` 会直接解析失败。

## 符号对照

| insgo | gohttpkit | 说明 |
|---|---|---|
| `internal.Client` / `internal.NewClient` | `httpx.Client` / `interceptor.NewClient(Options)` | 去掉 Platform / VersionConfig，换成 `HeaderProvider` |
| `Client.DoRequestWithHeadersAndWhitelist` | `Client.Do(ctx, RequestSpec)` | 7 个位置参数改成结构体 |
| `Client.GetWithHeadersAndWhitelist` | `Client.Get` / `Client.Do` | |
| `Client.PostFormWithHeadersAndWhitelist` | `Client.PostForm` / `Client.Do` | |
| `internal.NewInterceptorChain` | `interceptor.DefaultChain` | 去掉 IG 专属三层 |
| `internal.NewRawInterceptorChain` | `interceptor.DefaultChain` | 新库默认就是 raw，不再需要独立预设 |
| `internal.NewWarmupChain` | `interceptor.DefaultChain` | |
| `internal.NewWarmupNoRedirectChain` | `interceptor.NoRedirectChain` | |
| `internal.NewWarmupStep1Chain` | `interceptor.NoRedirectChain` + `interceptor.NewRequestMutatorInterceptor`（删 cookie 头） | 删头属业务，示例见 `examples/fidelity` |
| `internal.InterceptorChain` | `httpx.Interceptors` | 切片类型；`httpx.Chain` 现在指链的执行游标 |
| `internal.Interceptor`（不可外部实现） | `httpx.Interceptor`（**可外部实现**） | 最大的一处变化，见下文 |
| `internal.UserInterceptor` / `pkg.Interceptor` | `httpx.Interceptor` | 只读面板与内部接口合并成一个 |
| `internal.HTTPTransaction` | `httpx.Transaction` | 多了 `duration_ms` 字段 |
| `internal.NewHTTPTransactionInterceptor` | `interceptor.NewTransactionInterceptor` | |
| `internal.NewHTMLSaveInterceptor` | `interceptor.NewHTMLSaveInterceptor` | |
| `internal.FilterHeadersByWhitelist` | `httpx.FilterHeadersByWhitelist` | 行为一致（含小写快路径优化） |
| `internal.DecodeResponse` | `httpx.DecodeResponse` | |
| `internal.TruncateBodyForLog` / `LogBodyMaxBytes` | `httpx.TruncateBodyForLog` / `httpx.LogBodyMaxBytes` | |
| `internal.ApplyProxyToTransport` | `netproxy.ApplyProxyToTransport` | |
| `internal.ParseProxyURL` / `DialContextWithProxy` | `netproxy.*` | |
| `pkg/traffic` | `traffic` | 原样 |
| `insgo/logger` | `logger` | 日志用 `SetConfig` / `SetHandler`，不读环境变量 |
| `insgo/errors`（通用半边） | `errors` | 只带 `RetryableError` / `IsRetryableNetworkError` / 关键词表 |
| `insgo/errors`（IG sentinel、`ClassifyIGResponse*`） | 不带 | 业务归类改由 `interceptor.NewClassifyInterceptor` 注入 |
| `internal/geo` | `geo` | 原样（含两张国家表与 keyset 守护测试） |
| `pkg/versions` | `versionreg` | 只取注册表 / 白名单 / Builder 模式，零 IG 数据 |
| `internal.HeaderBuilder` / `HeaderConfig` | 不带 | 纯 IG；改为自己实现 `httpx.HeaderProvider`，示例见 `examples/fidelity` |
| `internal/browser`、`internal/websocket`、`wsspool`、`pkg/uagen` | 不带 | 本次范围之外（uagen 还会拖回私有模块依赖） |

## 行为差异（重要）

迁移时最容易踩的几处。

### 1. 拦截器接口从「禁止外部实现」变成「完全公开」

insgo 刻意把 `intercept(ch *chain)` 设为包私有，外部只能用预设链。本库反过来：
`Interceptor` / `Chain` / `Request` / `Response` 全部导出且字段可写。

代价是链结构成为对外契约，收益是接入方能把自己的横切逻辑插进任意位置 ——
对一个通用基建库来说这笔交易必须做。

### 2. `HeaderWhitelist` 的 nil 与空 map 现在语义不同

| | insgo | gohttpkit |
|---|---|---|
| `nil` | 一个头都不发 | **发全量构建头** |
| `map[string]string{}` | 一个头都不发 | 一个头都不发 |

insgo 把两者混为一谈（因为它的每个端点都必配白名单）。通用库里「没传白名单」的自然语义
应当是「按构头结果发」，否则随手 `client.Get` 会得到一个连 `accept` 都没有的裸请求。
**迁移时凡是显式传 nil 期望「不发头」的地方，改成传空 map。**

### 3. 默认链不再做 IG 的三件事

insgo 默认链含 `igErrorClassify`（业务错误归类）、`igState`（`ig-set-*` 回写）、
`htmlText`（HTML 提纯）、`statusSemantics`（"Please wait" / 572 限流）。本库全部下沉为可选：

| insgo 那一层 | 现在怎么做 |
|---|---|
| `igErrorClassify` | `interceptor.NewClassifyInterceptor(func(status int, body []byte) error)` |
| `igState` 的响应头缓存 | 已在默认链里（`interceptor.NewResponseHeaderCacheInterceptor`） |
| `igState` 的 `ig-set-*` 回写 | `Options.OnResponseHeaders` |
| `htmlText` | `interceptor.NewHTMLTextInterceptor(errorPageMarkers...)` |
| `statusSemantics` | `interceptor.NewStatusSemanticsInterceptor(rule)`，限流文案用 `httpx.RetryableTextRule` |

顺带：insgo 里「默认链 vs raw 链」的二分在本库消失了 —— 默认就是 raw。

### 4. 修掉了 HTTP/1.1 下的重复头与默认 UA 泄漏

insgo 全小写写头的做法在 HTTP/2 上没问题（标准库 h2 用大小写不敏感比较），
但在 HTTP/1.1 下标准库按**规范化大写** key 识别 `User-Agent` / `Host` / `Content-Length`，
于是会发出重复头，并额外泄漏一条 `User-Agent: Go-http-client/1.1`。

insgo 因为只打 HTTP/2 的目标，这个问题一直没暴露。本库的 `applySpecialHeaders` 统一处理了这三个头，
使两种协议下产出同一份、不重复的线上字节。严格白名单模式下还会连标准库默认 UA 一起抑制。

### 5. 状态码与响应头的缓存时机对齐了

insgo 里 `statusCodeCache` 在 `bodyDecode` 之内、响应头缓存在之外，于是「解码失败时状态码已缓存、
响应头未缓存」（源码里标注为「怪异点，保留」）。本库把两者并排放在同一层级，时机一致。

### 6. 请求头快照的取点从 bridge 移到了终端

insgo 在 bridge 里 `req.reqHeaders = httpReq.Header`（别名同一 map）。本库在终端拦截器里取，
并做两处修正：剔除用于抑制默认 UA 的空值 `User-Agent`（它不在线上）、补回 `host`（它在线上）。
好处是插在终端之前的改写层（签名、删头）会被如实记录到日志与整包快照里。

自定义终端拦截器需要自己调一次 `httpx.SnapshotRequestHeaders(ch.Request())`。

### 7. 请求体编码支持面变大

insgo 只接受 `nil` / `string` / `url.Values` / `map`，其它类型报 `unsupported body type`。
本库额外接受 `[]byte` 和任意可 JSON 序列化的值（struct 等）。`string` 保序语义不变。

### 8. 重试策略可注入

insgo 的重试次数与退避只能靠 env 调。本库只认 `Options.Retry`（含自定义 `IsRetryable`），
不读环境变量；`httpx.NoRetry()` 可关掉重试。超时同样只认 `Options.Timeout` /
`Options.ResponseHeaderTimeout`。

`Options.Retry` 的类型是 `*RetryPolicy`（指针）：nil = 没配、走代码默认值；非 nil = 每个字段字面生效。
用指针而不是「值类型 + 零值即未配」，是因为后者分不清「没配」和「明确要求 MaxRetries=0」。

## 同步共性修复时

两边都改的典型是：`errors` 的可重试关键词表（新的代理/网络栈错误文案）、
`geo` 的国家→locale/时区表、transport 调优参数。
本库这三处与 insgo 保持逐行一致，同步时可直接对拷。
