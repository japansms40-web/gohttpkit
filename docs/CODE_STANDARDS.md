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
  范例：`httpx/presets.go` 的 `DefaultChain` 注释解释了每一层为什么在那个位置。
- **MUST** 业务判断不进默认链。任何「替调用方对响应下结论」的逻辑都必须是可选拦截器。
  范例：`httpx/interceptors_opt.go` 全文件。
- **SHOULD** 大数据映射表（国家→locale、国家→时区）独立成文件，表头写明数据源与维护规约。
  范例：`geo/locale.go`、`geo/timezone.go`；两表 keyset 由测试守护。

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

- **MUST** 包装错误一律用 `%w`，判定一律用 `errors.Is` / `errors.As`（`errorlint` 硬卡）。
- **MUST** 网络发送失败必须包成 `*httpx.TransportError`，否则重试层看不见它。
- **SHOULD** 环境变量非法值回落默认而不是报错终止 —— 它是运维旋钮，打错一个字母不该让进程起不来；
  但也绝不能静默变成 0（对超时类配置而言 0 意味着关闭保护）。范例：`internal/envx.Int`。

## 6. 日志

- **MUST** 统一走 `logger` 门面（`logger.Info(ctx, msg, attrs...)`），ctx 必传首参。
  `forbidigo` 会拦截裸 `fmt.Print` / `log.*` / `slog.*`（`logger/` 包自身与 `examples/` 除外）。
- **MUST** 打协议 body 时经 `TruncateBodyForLog(b, client.LogBodyLimit())` 截断，
  并同时输出原始长度（`slog.Int("..._len", len(b))`）。响应体可达数百 KB，
  全量打印会撑爆磁盘与日志聚合系统。

## 7. 注释

- **MUST** 注释解释「为什么」，不复述「是什么」。尤其是那些看起来可以简化、实际上不能动的地方
  —— 写明它踩过什么坑，否则下一个人会「顺手优化掉」。
  范例：`httpx/transport.go` 里每个 transport 参数的注释、`applySpecialHeaders` 的整段说明。
- **MUST** 行为怪异但有意保留的地方显式标注，并在 characterization 测试里锁定。

## 8. 测试

- **MUST** 对外行为的改动必须有 characterization 测试覆盖。测的是行为（重试几次、发哪些头、
  什么时机缓存），不是实现细节。范例：`httpx/characterization_test.go`。
- **SHOULD** 用例名写成中文短句，直接说明它锁的是什么行为，失败时不用读代码就知道坏了什么。
- **SHOULD** 需要网络的测试自带假服务器（`httptest` / 最小协议实现），CI 里不依赖外网。
  范例：`netproxy/proxy_test.go` 里的最小 SOCKS5 服务端。
