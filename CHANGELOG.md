# 更新日志

格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [`docs/VERSIONING.md`](docs/VERSIONING.md)，
发布流程见 [`docs/RELEASE.md`](docs/RELEASE.md)。每个版本按「破坏 / 新增 / 变更 / 修复 / 弃用 / 安全」分组，空组省略。

## [Unreleased]

### 新增

- `logger.Named(name) *Logger`：不需要 ctx 的具名日志句柄，`Debug/Info/Warn/Error(msg, attrs...)` 直接打，
  name 非空时每条带 `module=name`；`Logger.With(attrs...)` 追加固定字段（copy-on-write）。每次调用才读当前全局 handler，
  包级 var 先于 `SetHandler` / `SetConfig` 创建也生效；零值与 nil 接收者可用。不带 trace_id / span_id，要同链仍用 `logger.Info(ctx, ...)`。
- `logger.FieldModule`（`"module"`）：`Named` 写入的字段 key，列入保留字段。

## [v0.7.0] - 2026-09-26

> 以 `tools/agentguard` 开头的条目只涉及仓库工具（独立子模块，按 `tools/agentguard/vX.Y.Z` 单独打 tag），不影响库的导出 API；
> 本版的 agentguard 修复随 `tools/agentguard/v0.1.3` 发布。

### 变更

- `interceptor.NewRetryInterceptor`：发送失败正由调用方 ctx 结束引起时（`req.Ctx.Err()` 非空且错误链含该 ctx 错误）不再重试、
  不再打 `http.retry` 告警，返回 `failed to send request: %w`（可 `errors.Is` 判 `context.DeadlineExceeded` / `Canceled`），
  不再包成 `*errors.RetryableError`。此前 `"context deadline exceeded"` 命中可重试关键词，调用方超时会被当成网络抖动。
  `http.Client.Timeout` 引起的超时（调用方 ctx 仍存活）照常重试；可重试失败后在退避中取消仍返回 `*errors.RetryableError`，不变。

### 修复

- `tools/agentguard`：git 子命令改用 `exec.CommandContext` 并加 1 分钟超时，git 卡在锁或凭据提示时钩子不再无限等待。

## [v0.6.0] - 2026-09-25

> 以 `tools/agentguard` 开头的条目只涉及仓库工具（独立子模块，按 `tools/agentguard/vX.Y.Z` 单独打 tag），不影响库的导出 API。
> 其中「`.agentguard.yml` 声明」随 `tools/agentguard/v0.1.0` 发布，「`git config` 区分读写」随 `v0.1.1` 发布。

### 新增

- `httpx.StatusRuleContext` 与 `interceptor.NewStatusSemanticsInterceptorContext`：状态语义规则额外拿到本次请求的 ctx，
  命中时可打带 trace_id 的事件日志（原 `StatusRule` 拿不到 ctx，接入方只能自写拦截器）。
  `NewStatusSemanticsInterceptor` 签名与行为不变；nil 规则同样回落 `DefaultStatusRule`。
- `tools/agentguard`：治理基线支持主干不叫 `main` 的仓库：取值顺序改为 `--base` / `$GOVERNANCE_BASE` → `merge-base(HEAD, origin/HEAD)` →
  `merge-base(HEAD, origin/<main_branch>)` → `merge-base(HEAD, origin/main)` → `HEAD`；`.agentguard.yml` 新增 `main_branch`（默认 `main`，从 HEAD 提交读取）。
- `tools/agentguard`：仓库差异改由仓库根 `.agentguard.yml` 声明（`characterization.file` / `func_pattern`，读基线上的配置），
  供其它仓库按版本 `go install` 复用，不再复制源码；char 检查支持只看命中 `func_pattern` 的用例函数（按行号范围判定）。

### 修复

- `tools/agentguard`：`git config` 区分读写，`--get` / `get` / 单键无值读取 `core.hooksPath` 不再被当成改写拦下；
  补拦 `--remove-section core` / `--rename-section core …`（会连带删除 hooksPath）。

## [v0.5.0] - 2026-09-24

### 破坏

- 删除 `interceptor.NewClient`，改用 `httpx.NewClient(httpx.Options{...})`：Interceptors 为 nil 时装默认链，空切片仍视为显式自组链。
  默认链由 `httpx/interceptor` 包在 `init()` 里经 `httpx.RegisterDefaultChain` 注册，程序里需 import 一次该包
  （用到任何拦截器即已满足，否则 blank import）；未注册时 `httpx.NewClient` 返回 `*httpx.NoDefaultChainError`。
  迁移：把 `interceptor.NewClient(` 替换为 `httpx.NewClient(`，确认仍 import 了 `httpx/interceptor`。

### 新增

- `httpx.NewClient`、`httpx.RegisterDefaultChain`、`httpx.NoDefaultChainError`。

## [v0.4.1] - 2026-09-23

### 变更

- 仓库工作流：改代码走 `git worktree`，提交与打 tag 直接在 `main` 上做；agent 钩子不再拒绝 `main` 上的提交，放行新建 tag，
  删除 / 移动 / 推送 tag 仍拒绝（仓库工具，不影响库的导出 API）。

## [v0.4.0] - 2026-09-23

### 破坏

- 最低 Go 版本从 1.25.0 上调到 1.27.1（仓库工具 `tools/agentguard` 从 1.24.0 同步上调）。使用本库的模块须升级到 Go 1.27.1 或更高才能编译。CI 改为 `go-version: stable`，golangci-lint 改用 `latest`（`golangci-lint-action` 升到 v9）。

### 安全

- 直接依赖升级：`golang.org/x/net` v0.55.0 → v0.59.0、`github.com/andybalholm/brotli` v1.2.1 → v1.2.4、`github.com/klauspost/compress` v1.18.6 → v1.20.0。其中 `x/net` 修复经 `httpx.ExtractHTMLText` → `html.Parse` 可达的 HTML 解析漏洞，以及 HTTP/2 相关漏洞。

## [v0.3.3] - 2026-09-23

### 修复

- CI 固定 golangci-lint v2.12.2，兼容 Go 1.25（仓库门禁，不影响库本身）。

## [v0.3.2] - 2026-09-23

> 注：本版本把 `go` 指令从 1.24.0 上调到 1.25.0，按 `docs/VERSIONING.md` 应为 minor；tag 已发布，此处如实补记。

### 破坏

- 最低 Go 版本从 1.24.0 上调到 1.25.0。

### 新增

- 规范：`docs/RELEASE.md`、`SECURITY.md`、本文件；`CODE_STANDARDS` §10–16、`TESTING` §8–11；AI 代理硬性纪律。
- 门禁：`nolintlint`；CI 显式 `make char`；治理守卫 `make governance`（CI / pre-push / agent 收尾）；
  Claude / Cursor / Codex 三家 agent 钩子（`tools/agentguard`，仓库工具，不影响库的导出 API），只有放行 / 拒绝两档。

### 安全

- `golang.org/x/net` v0.50.0 → v0.55.0。

## [v0.3.1] - 2026-09-23

> 注：本版本新增了导出 API，按 `docs/RELEASE.md` §2 应为 minor（`VERSIONING.md` 原计划记为 v0.4.0）。

### 新增

- 子包 `geo/locale_mobile`（package `localemobile`）：`AcceptLanguageForCountry`、`AppLocaleForCountry`、
  `DeviceLocaleForCountry`、`MappedLocaleForCountry`、`DeviceLanguagesForCountry`，未命中返回 `*geo.UnknownCountryError`。

### 修复

- `httpx`：重试退避翻倍时整型溢出会绕过 `MaxBackoff` 封顶，现已钳住。

## [v0.3.0] - 2026-09-22

### 破坏

- `httpx.ContentEncodingError.Encoding`、`httpx.ReadResponseBodyError.Encoding` 由 `string` 改为 `httpx.ContentEncoding`。

### 新增

- `httpx.ContentEncoding` / `ParseContentEncoding`；`httpx.ReadResponseBodyError.RawEncoding`。
- `httpx.Header*` header 名 const、`httpx.LogField*` / `logger.Field*` 日志字段 key const。
- `httpx.StatusClass` / `ClassifyStatus` / `IsSuccessStatus` / `IsErrorStatus`。
- 内建拦截器拆入 `httpx/interceptor` 子包；httpx 类型错误可 `errors.As` 读字段。

### 修复

- `Transaction` 快照深拷贝 Header。

## [v0.2.0] - 2026-08-26

### 破坏

- `Options.Retry` 改为 `*RetryPolicy`：nil 表示走默认策略，非 nil 字面生效（`MaxRetries: 0` 即不重试）。

### 新增

- 全包测试覆盖到 98.1%，加 90% 覆盖率门禁（此后逐步上调，现为 98%）。

## [v0.1.0] - 2026-08-25

### 新增

- 初版：`httpx`、`geo`、`logger`、`netproxy`、`traffic`、`versionreg`、`errors`。
