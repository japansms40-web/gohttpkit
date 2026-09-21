# gohttpkit

一套可直接复用的 Go HTTP 客户端基建：**OkHttp 风格的拦截器链**、网络层重试与指数退避、
四种压缩解码、代理接入与 TCP 层流量计数、trace_id 贯通的结构化日志、
**header 白名单精确发头**、请求/响应整包快照落盘。

它不绑定任何具体 API。请求头怎么构建由你实现 `HeaderProvider` 决定，业务错误怎么归类由你写一层
拦截器决定 —— 库只负责把这些横切关注点组织成一条可预测、可测试、可扩展的链。

```bash
go get github.com/japansms40-web/gohttpkit
```

依赖只有三个，全是公开模块，没有任何 `replace`：`brotli`、`klauspost/compress`、`golang.org/x/net`。
最低 Go 版本 1.24。

---

## 30 秒上手

```go
client, err := interceptor.NewClient(httpx.Options{
    Headers: httpx.StaticHeaders{
        Base:    "https://api.example.com",
        Headers: map[string]string{"accept": "application/json"},
    },
})
if err != nil { return err }

body, err := client.Get(ctx, "/v1/ping", nil)
status := client.SnapshotResponseStatusCode()
```

这样就已经有了：自动重试、自动解压、结构化日志、响应头缓存。

```bash
go run ./examples/quickstart                    # 打一个真实接口
go run ./examples/customchain                   # 自定义拦截器（签名/计数/回写/归类）
go run ./examples/fidelity                      # 高保真复刻抓包
```

---

## 包一览

| 包 | 作用 |
|---|---|
| `httpx` | **核心框架**：Client、拦截器链框架、白名单发头、`Transaction` 快照类型；内建拦截器（重试/解压/日志/缓存）与默认链在子包 `httpx/interceptor` |
| `logger` | 基于 `log/slog` 的日志门面，trace_id / span 自动注入，可换成你自家的 handler |
| `errors` | 可重试网络错误的判定与包装（关键词表可扩展）、HTTP 状态码错误 |
| `netproxy` | socks5 / http / https 代理接入 `http.Transport`，或拿裸 `proxy.Dialer` 给 TCP 链路用 |
| `traffic` | TCP 层真实收发字节计数，零注入时零开销，贴近代理商计费口径 |
| `geo` | 国家码 → Web data-code / Android locale / 时区，代理 URL → 出口国，按 Chromium 拼 Accept-Language |
| `versionreg` | 「按版本隔离协议实现」的泛型注册表骨架 + 按 endpoint 的白名单访问器 |

---

## 三个值得先理解的设计取舍

### 1. 默认链不替你做业务判断

```
logging → bodyDecode → statusCodeCache → responseHeaderCache → retry → bridge → callServer
```

默认链**不**提纯 HTML、**不**归类业务错误、**不**把非 2xx 变成 error。
非 2xx 的响应体原样返回给你，状态码走 `SnapshotResponseStatusCode()` 读。

理由：大量私有 API 用 4xx 承载有意义的业务响应体；基建层替你判死会丢信息。
需要这些行为时显式加一层，加什么、加在哪，你说了算：

```go
chain := httpx.Prepend(interceptor.DefaultChain(),
    interceptor.NewClassifyInterceptor(myClassify),        // 业务错误 → sentinel error
    interceptor.NewStatusSemanticsInterceptor(nil),        // 非 2xx 空体 → error
)
chain = httpx.SpliceBeforeTerminal(chain, mySigner)  // 请求签名，包住每次真实发送
```

`interceptor.APIChain(classify)` 是这两层的现成组合。

### 2. `HeaderWhitelist` 的 nil 与空 map 不是一回事

```go
client.Get(ctx, "/x", nil)                                     // nil → 发全量构建头
client.Do(ctx, httpx.RequestSpec{Path: "/x",
    HeaderWhitelist: map[string]string{"accept": ""}})          // 严格：只发 accept
client.Do(ctx, httpx.RequestSpec{Path: "/x",
    HeaderWhitelist: map[string]string{}})                      // 严格：一个都不发
```

白名单里 value 为空串表示「取构建值」，非空表示「用这个固定值覆盖」。
严格模式下连标准库的默认 `User-Agent` 都会被抑制 ——「我告诉你发哪些头」就该字面成立。

### 3. 重试策略：nil 是「没配」，不是「不重试」

```go
interceptor.NewClient(httpx.Options{Headers: hp})                          // 默认 3 次 + 200/400/800ms
interceptor.NewClient(httpx.Options{Headers: hp, Retry: httpx.NoRetry()})  // 明确不重试
interceptor.NewClient(httpx.Options{Headers: hp,
    Retry: httpx.WithRetry(5, 100*time.Millisecond, time.Second)})
```

`Options.Retry` 是**指针**：nil 走代码默认值，非 nil 则每个字段字面生效 ——
包括 `MaxRetries: 0`（就是不重试）。用值类型加「零值即未配」的话，
分不清「没配」和「明确要求不重试」，后者会被静默改成重试 3 次。

只有 `*TransportError`（网络发送本身失败）会触发重试；HTTP 状态码不会 ——
「503 该不该重试」是业务语义，自己写一层拦截器，别让基建替你决定。

### 4. 会话状态回写挂在 Options 上，不挂在链上

```go
interceptor.NewClient(httpx.Options{
    Headers: myProvider,
    OnResponseHeaders: func(ctx context.Context, h http.Header) {
        myProvider.ApplySetCookie(h)   // 服务端下发的新 token 立刻回写
    },
})
```

它是**会话级**而非链级的责任，所以 `WithChain` 派生子客户端时不会失效。
漏掉这一步的典型症状是「全程 200、业务就是不成功」—— 因为你一直在用过期凭证。

---

## 高保真复刻

如果你在复刻一个真实客户端的请求，这几件事决定成败（`examples/fidelity` 逐条演示）：

- **只发抓包里出现过的头**：用 `HeaderWhitelist` 精确控制，多一个头就可能被识别为非官方客户端
- **头名全小写**：真实 HTTP/2 客户端发的就是小写，`Sec-Ch-Ua` 这种大写形态是明显的机器特征。
  本库绕过标准库的规范化直写底层 map，并额外处理了 `host` / `content-length` / `user-agent`
  这三个标准库特殊对待的头，让 HTTP/1.1 与 HTTP/2 下产出同一份、不重复的线上字节
- **cookie 手工按抓包顺序拼**：map 遍历顺序随机，顺序不稳定本身就是特征
- **表单参数保持抓包顺序**：`url.Values.Encode()` 会按字典序重排，传 `string` 才能保序
- **整包落盘事后对比**：挂 `interceptor.NewTransactionInterceptor`，每次请求产出完整快照 JSON（头是原文）

**已知边界**：Go 的 `http.Header` 是 map，本库无法控制头在线上的**顺序**，只能保证
「发哪些头、值是什么、大小写如何」。绝大多数服务端不校验头顺序；若你的目标真的校验，
需要自定义 Transport 直接写 HTTP/2 帧，超出本库范围。

---

## 写一个自己的拦截器

```go
type signer struct{ key []byte }

func (s *signer) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
    req := ch.Request()                       // 可读可改：URL、body、头、Attempt
    req.HTTPReq.Header["x-sig"] = []string{sign(s.key, req)}
    return ch.Proceed()                        // 继续链；可多次调用（重试层就是这么做的）
}
```

插入位置决定语义：

| 位置 | 用 | 看得到 |
|---|---|---|
| `Prepend`（最外层） | 业务归类、状态语义、埋点 | 最终响应体，一次调用只走一遍 |
| `SpliceBeforeTerminal`（终端之前） | 签名、限速、计费、删头 | 已构建好的 `*http.Request`，**每次重试各一遍** |
| `Replace(chain, IsTerminal, x)` | 录制回放、自定义传输 | 自己负责发请求（错误要包成 `*TransportError` 才会被重试） |

旁路观察层（只落盘不改语义）内嵌 `httpx.SideChannelMarker`，`WithChain` 派生子链时会自动带上。

---

## 超时与重试

超时、重试、慢请求只认 `Options`，不读环境变量。零值回落代码默认：整请求 30s、响应头 15s、重试 3 次。

```go
client, err := interceptor.NewClient(httpx.Options{
    Headers:               headers,
    Timeout:               10 * time.Second,
    ResponseHeaderTimeout: 5 * time.Second,
    SlowMS:                800 * time.Millisecond,
    Retry:                 httpx.WithRetry(5, 100*time.Millisecond, time.Second),
})
```

关掉重试用 `httpx.NoRetry()`。日志用 `logger.SetConfig` / `logger.SetHandler`，也不读环境变量。

按 `event=http.transaction` 过滤交易日志，请求头与响应头按原文输出。
高保真复刻用的 `Transaction` 快照同样是原文（给抓包对比），不要写进共享日志。

---

## 并发模型

- 单个 `*Client` 可被多 goroutine 并发调用；「最近一次响应」的状态码与响应头受锁保护，
  外部读取必须走 `SnapshotResponseStatusCode()` / `SnapshotResponseHeaders()`，不要直读字段
- `HeaderProvider` 的并发安全**由你的实现保证** —— 带会话状态的实现请自带锁（见 `examples/fidelity`）
- 跨会话不要共享 `Client`：它与 `HeaderProvider` 绑定，而后者通常携带某个账号的身份，复用会串号
- `logger` 是**进程级全局单例**（`SetHandler` 会影响整个进程）。多个库共用时注意互相覆盖

---

## 开发

贡献流程与提交前清单见 [`CONTRIBUTING.md`](CONTRIBUTING.md)；版本策略（导出即契约，0.x 破坏性变更升 minor）见 [`docs/VERSIONING.md`](docs/VERSIONING.md)。

```bash
make check       # 本地轻量：build + vet + cover(含 90% 门禁) + tidy-check
make ci          # 合入口径聚合：check + lint-new + race + char（race 需要 C 编译器）
make char        # 行为锁定套件：改拦截器链之前先跑它
make cover       # 覆盖率报告 + 门禁；make cover-html 看逐行
make race        # 并发回归（需要 C 编译器）
make examples    # 跑两个离线示例（customchain / fidelity）；quickstart 请求真实 URL，需外网，单独跑
```

合入前请跑 `make check`，以及 `make lint-new` 与 `make race`（CI 已覆盖这两项）。想一条命令跑合入门禁，用 `make ci`。注意与 CI 的差异：CI 上 `lint` 是**全量** `golangci-lint run`，本地 `lint-new` 只查相对 `BASE_REV` 的增量；`make ci` 不跑 examples，CI 会另跑 `customchain` / `fidelity`。

覆盖率门禁在 `make cover` 里，低于 90% 直接失败（`MIN_COVERAGE` 可调高，不要调低）。
当前总覆盖率 **95.0%**（≥ 90% 门禁）。多数包在 97% 以上，`httpx/interceptor` 目前 80.3%，是后续要补测的重点：

| 包 | 覆盖率 | | 包 | 覆盖率 |
|---|---|---|---|---|
| `errors` | 100% | | `httpx` | 99.3% |
| `traffic` | 100% | | `logger` | 99.4% |
| `versionreg` | 100% | | `netproxy` | 97.1% |
| `geo` | 100% | | `examples/*` | 93~96% |
| `httpx/interceptor` | 80.3% | | | |

测试分三层，各管各的：

- `httpx/characterization_test.go` —— **对外行为**锁定：重试几次、退避多久、哪些错误不重试、
  白名单怎么过滤、头的大小写、缓存时机、链的执行顺序。链的顺序一旦被改动，破坏的往往不是编译，
  而是某个只在生产环境偶发的行为 —— 那就是它存在的理由。
- 各源文件同名的 `*_test.go`（`httpx/*_test.go` 与 `httpx/interceptor/*_test.go`）—— 每个导出符号自己的契约：
  编解码矩阵、选项归一化、链编辑工具、各类 nil / 零值 / 错误分支。
- `httpx/concurrency_test.go` —— 并发回归，配合 `make race` 用。

示例也在测试里跑（`examples/*/main_test.go`）：示例是给人读的，但读者会照抄，
所以它必须真的能跑，且行为如注释所述。

## 血缘

本库的实现从一个生产中的私有 API 客户端项目（insgo）里抽取并彻底去业务化而来，
其中的每一条 transport 参数、每一个重试关键词、每一处「怪异但必要」的行为都带着实战注释。
对照表见 [`docs/MIGRATION_FROM_INSGO.md`](docs/MIGRATION_FROM_INSGO.md)。

## 许可证

MIT
