# gohttpkit 代码规范

> 每条都附本仓库内的范例文件。规范与代码不一致时，以规范为准修代码，或提 PR 修订规范，
> 禁止二者长期背离。

## 1. 效力分级

- **MUST**：违反即打回，CI / 评审硬卡。
- **SHOULD**：默认遵守，违反需在 PR 描述或代码注释里给出理由。
- **MAY**：推荐做法，自行裁量。

质量门禁用 `make check`；增量严格用 `make lint-new`（`golangci-lint --new-from-rev`）。

## 2. 包设计

- **MUST** 本库是给别人用的基建，导出即契约。新符号默认小写；确需导出的，doc comment
  必须写明「给谁用、什么场景」。范例：`httpx.SnapshotRequestHeaders`（注明自定义终端需自行调用）。
- **MUST** 拦截器链的顺序即语义。调整顺序前先跑 `make char`，并在 PR 里说明行为变化。
  范例：`httpx/interceptor/chain.go` 的 `DefaultChain` 注释解释了每一层为什么在那个位置。
- **MUST** 业务判断不进默认链。任何「替调用方对响应下结论」的逻辑都必须是可选拦截器。
  范例：`httpx/interceptor` 下的 classify / status_semantics / html_text。
- **SHOULD** 大数据映射表（国家→locale、国家→时区）独立成文件，表头写明数据源与维护规约。
  范例：`geo/locale_web.go`、`geo/locale_mobile.go`、`geo/timezone.go`；三表 keyset 由测试守护。

## 3. API 设计

- **MUST** 所有会发起 IO 的函数第一参是 `context.Context`。
- **MUST** 参数超过 2 个业务字段时用结构体，不写长参数列表。
  范例：`httpx.RequestSpec` 取代 insgo 的 7 参数位置调用。
- **MUST** 不导出私有类型别名到公开签名里（godoc 会显示一个外部拿不到的名字）。
- **SHOULD** 配置构建用 functional options；简单数据对象直接 struct 字面量，不为它写 Builder。
  范例：`versionreg/example_config.go`。
- **SHOULD** 返回给调用方的 map / slice 一律给副本，除非文档明确写了「只读，勿改」。
  范例：`versionreg.HeaderWhitelists.For`。

## 4. 并发模型

- **MUST** 含锁结构体的类型定义头部写并发模型注释：谁共享、哪把锁保护哪些字段。
  范例：`httpx.Client` 头注。
- **MUST** 受锁字段跨包只能经访问器读写，禁止裸读写绕过锁。
  范例：`Client.SnapshotResponseStatusCode` / `SnapshotResponseHeaders`。
- **MUST** `make race` 通过是合入门禁（需要 C 编译器）。并发回归必须配并发测试。
  范例：`httpx/concurrency_test.go`。
- **SHOULD** 锁内不做内存分配 / IO：先在锁外构建完整数据，再加锁整体替换引用（Snapshot 模式）。
  范例：`responseHeaderCacheInterceptor` 在锁外构建 `newHeaders` 再锁内换引用。

## 5. 错误处理

本库返回给调用方的错误是契约：必须能对比类型、读出字段。禁止用一句话哨兵冒充身份。

- **MUST** 本库错误定义为带字段的类型（`type XxxError struct` + `Error()`），直接 `return &XxxError{...}`。
  对比用 `errors.As` 解出类型和字段，不要扫 `Error()` 文案。
  范例：`geo.UnknownCountryError`、`geo.InvalidExtraLanguageCountError`、
  `geo.ExtraLanguageCountExceedsPoolError`、`errors.RetryableError`、`errors.HTTPStatusError`。
- **MUST NOT** 用 `errors.New` / `fmt.Errorf("...")`（无 `%w`）/ `var ErrXxx = errors.New(...)`
  作为本库错误的身份。哨兵没有字段，对比只能 `errors.Is` 或扫文案，`extra=3`、国家码、状态码都会丢。
  `fmt.Errorf("%w: ...", err)` 只允许包一层上下文，里层必须仍是上面的类型错误。
- **MUST** 包装错误一律用 `%w`（`errorlint` 硬卡）。
- **MUST** 对本库错误的测试用 `errors.As` 断言类型和字段，禁止 `errors.Is` 对自造哨兵、禁止比文案当身份。
  范例：`geo/errors_test.go` 的 `assertUnknownCountry` / `assertInvalidExtra` / `assertExtraExceedsPool`。
- **MUST** 网络发送失败必须包成 `*httpx.TransportError`，否则重试层看不见它。
- **MAY** `errors.Is` 只认本库没定义、对方已经是哨兵的错误（`io.EOF`、`context.Canceled`、第三方）。
  测试里用 `errors.New("boom")` 冒充「别人的错误」可以；本库自己的返回值不行。
- **SHOULD** 超时类 `Options` 的零值回落默认常量，禁止把 0 解释成「关闭保护」。
  范例：`Options.Timeout`、`Options.ResponseHeaderTimeout`。

```go
// 对
return "", &UnknownCountryError{Country: key}
var ue *UnknownCountryError
if errors.As(err, &ue) { use(ue.Country) }

// 错
var ErrUnknownCountry = errors.New("geo: unknown country")
return "", fmt.Errorf("%w: %s", ErrUnknownCountry, key) // 字段进了文案
errors.Is(err, ErrUnknownCountry)                       // 对比不到 Country
```

## 6. 日志

- **MUST** 生产代码统一走 `logger` 门面（`logger.Info(ctx, msg, attrs...)`），ctx 必传首参。
  `forbidigo` 会拦截裸 `fmt.Print` / `log.*` / `slog.*`（`logger/` 包自身与 `examples/` 除外）。
  测试观察日志走 `t.Log` / `t.Logf`，见第 8 节，不要在 `*_test.go` 里打 `logger` 或 `fmt.Print`。
- **MUST** 打协议 body 时经 `TruncateBodyForLog(b, client.LogBodyLimit())` 截断，
  并同时输出原始长度（`slog.Int("..._len", len(b))`）。响应体可达数百 KB，
  全量打印会撑爆磁盘与日志聚合系统。
- **MUST** `event=http.transaction` / `event=http.retry` 日志按原文打请求头与响应头，不做脱敏。
  `Transaction` 快照同样是原文，给抓包对比用，不要写进共享日志。

## 7. 注释

函数注释是契约。godoc 第一行必须以符号名开头。顺序固定：**做什么 → 输入 → 返回 → 例 → 为什么**。

- **MUST** 每个函数（含未导出）写清「输入什么、返回什么」，用 `输入：` / `返回：` 起行，方便扫读。
  - 输入：每个参数的含义、允许形态、空值 / 空白 / nil 怎么处理、会不会改调用方数据。
  - 返回：成功值长什么样；每一种失败或空结果对应哪个 error / bool / 零值。
  - 零值合法时必须写清怎么和失败区分，禁止让读者去读实现才知道。
  范例：`geo.TimezoneOffsetForCountry`（`0` 是合法 GMT+0，必须看 error）、
  `geo.lookupCountry`、`geo.BuildChromeAcceptLanguageForCountry`。
- **SHOULD** 导出函数再写一行 `例：调用 → 结果`，至少覆盖最常见成功路径和一条失败路径。
  范例：`geo.WebAcceptLanguageForCountry`、`geo.MobileLocaleForCountry`。
- **MUST** 导出符号还要写「给谁用、什么场景」（见第 2 节）。
- **MUST** 「为什么」写在输入 / 返回之后，不复述「是什么」。看起来能简化但不能动的地方，
  写明踩过什么坑，否则下一个人会「顺手优化掉」。
  范例：`httpx/transport.go` 的 transport 参数、`geo.expandChromeLanguageList` 的前瞻规则。
- **MUST** 行为怪异但有意保留的地方显式标注，并在 characterization 测试里锁定。
- **MUST NOT** 注释只复述函数名或实现步骤（「遍历切片然后返回」）。

## 8. 测试

- **MUST** 对外行为的改动必须有 characterization 测试覆盖。测的是行为（重试几次、发哪些头、
  什么时机缓存），不是实现细节。范例：`httpx/characterization_test.go`。
- **SHOULD** 用例名写成中文短句，直接说明它锁的是什么行为，失败时不用读代码就知道坏了什么。
- **SHOULD** 需要网络的测试自带假服务器（`httptest` / 最小协议实现），CI 里不依赖外网。
  范例：`netproxy/proxy_test.go` 里的最小 SOCKS5 服务端。
- **SHOULD** 测试文件与源文件同名：`foo.go` → `foo_test.go`。fuzz 用 `foo_fuzz_test.go`。
  范例：`geo/locale_web_test.go`、`netproxy/proxy_fuzz_test.go`。不写 `example_test.go`
  当说明书。

### 8.1 测试日志

审查要能看见每条用例的输入和结果，但不能让默认 `go test` / CI 刷屏。

- **MUST** 用 `t.Log` / `t.Logf`。禁止 `fmt.Print` / `log.*` / `slog.*` / `logger.*`。
- **MUST** 默认安静：不带 `-v` 时这些日志不出现。审查或对表时用 `go test -v ./包/`。
  只看查表：`go test -v ./geo/ -run TestWebAcceptLanguageForCountry`。
  只看拼装：`go test -v ./geo/ -run TestBuildChromeAcceptLanguage`。
- **SHOULD** 表驱动每条在断言前打一行，写清输入和结果：
  `t.Logf("country=%q → %q err=%v", country, got, err)`。
  范例：`geo/locale_web_test.go`、`geo/lookup_test.go`。
- **SHOULD** 全表扫描按 key 排序后再 `Log`，map 遍历顺序不稳定，排过才方便对表。
  范例：`geo/locale_web_test.go` 的 `TestCountryToWebAcceptTag_值不含下划线`。
- **MAY** 错误路径再打 `errors.As` 解出的类型和字段，方便核对类型错误而不是文案。
  范例：`geo/errors_test.go` 的 `assertUnknownCountry` / `assertInvalidExtra` /
  `assertExtraExceedsPool`。本库错误的定义与对比见第 5 节。

## 9. 常量、枚举与魔法值

稳定取值是对外契约：下游按日志字段做过滤、按 header 名复刻指纹、按 encoding 分支。
散落字面量意味着改一个名字要全仓库搜，漏一处就静默出错。先判定该用枚举还是具名 const，
再写代码；禁止为了消灭字面量而无脑加 `type`。

- **MUST** 闭合取值集合，且会被 `switch` / 比较 / 解析或带方法的，定义为 `type X string`
  （或其它合适底层类型）枚举 + 常量 + `String()`；需要从外部原文收回来时再加 `ParseX`。
  范例：`netproxy.Scheme`、`logger.Level` / `logger.Format` / `logger.Output`、
  `httpx.ContentEncoding`、`httpx.StatusClass`。
- **MUST** 对外契约类稳定标识符（HTTP header 名、结构化日志字段 key、event / span 名）
  集中定义为具名 const，单一事实源；禁止在分支、拼装、打点处再写同名字面量。
  范例：`httpx/header_names.go`、`httpx/log_fields.go`、`logger/fields.go`、
  `httpx.EventHTTP*` / `httpx.SpanHTTPRequest`。
- **SHOULD** header 名、日志字段 key 用具名 const，**不要**套 `type` 枚举。
  它们作 `map[string]string` 的 key 或 `slog.String(key, v)` 的实参，套 type 只在每个
  访问点引入 `string(...)` 转换噪音，没有分支收益。标准库 `net/http` 对 header 名也是
  纯 const。判定边界：值会进 `switch` → 枚举（如 `content-encoding` 的 gzip/zstd）；
  只当 map key / 日志字段名 → const（如 `"content-length"`、`"trace_id"`）。
- **MUST** 数值边界（HTTP 状态码等）用 stdlib 常量（`http.StatusBadRequest`）或本库
  `StatusClass` / `ClassifyStatus` / `IsSuccessStatus` 表达语义，禁止裸区间魔法数字
  （如 `code >= 200 && code < 300`、`code >= 400`）。
- **明确豁免**：
  - `geo/` 大数据映射表里的 `"Hans"` / `"zh-CN"` 等字面量就该写在每一行
    （`.golangci.yml` 已豁免 `goconst`）。表即数据，提成常量反而看不出表长什么样。
  - `examples/` 为直观允许字面量（`.golangci.yml` 已豁免）。
  - `*_test.go` 可用字面量从「外部视角」核对契约实际值；生产代码不得回写字面量。
