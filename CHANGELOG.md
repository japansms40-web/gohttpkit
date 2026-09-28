# 更新日志

格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [`docs/VERSIONING.md`](docs/VERSIONING.md)，
发布流程见 [`docs/RELEASE.md`](docs/RELEASE.md)。每个版本按「破坏 / 新增 / 变更 / 修复 / 弃用 / 安全」分组，空组省略。

## [Unreleased]

### 新增

- `versionreg` 导出 `KindRegisterInvalid`、`KindRegisterEmptyID`、`KindRegisterDuplicate`，
  recover `MustRegister` 的 panic 后可直接 `errors.IsKind(err, versionreg.KindRegisterDuplicate)`，不必手写分类名；名称与 v0.11.0 一致。
- `make check-errors`（`tools/agentguard`）新增三条规则，落实 `docs/CODE_STANDARDS.md` §5.1：
  `errors.Error` 字面量的 `Op` 须引用具名 const；`errors.NewKind` 只能出现在包级 `var` 声明里；
  Kind 名须为 `<包>.<分类>` 小写点分字符串字面量。测试文件豁免。下游升级 agentguard 前需先按此整改。

### 变更

- 外层 `*errors.Error` 的 `Op` 统一为 `<包>.<步骤>`：`httpx.New` 自建 transport 失败由 `"httpx: build transport"`
  改为 `"httpx.build_transport"`；重试拦截器发送失败由 `"failed to send request"` 改为 `"interceptor.send_request"`。
  `Error()` 前缀随之变化；错误类型、内层原因与 `errors.As` / `errors.Is` 判定不变。按文案前缀匹配的调用方需改为按错误链判定。

### 安全

- 示例 `examples/quickstart` 的 `-url` 解析失败时不再保留 `url.Parse` 的错误：原错误会带出原始 URL，
  漏写 `@` 时还会把密码当端口回显（`invalid port ":secret"`）。现只返回 `quickstart.invalid_url` 分类与 `reason=parse`。

## [v0.11.0] - 2026-09-28

### 破坏

- `versionreg.Registry.MustRegister` 的校验失败、空标识、重复注册现在均 panic `*errors.Error`，分别带
  `versionreg.register.invalid`、`versionreg.register.empty_id`、`versionreg.register.duplicate` 分类。
  原来对空标识或重复注册使用 `recover().(string)` 的调用方，改为将恢复值断言为 `error`，再用
  `errors.As` / `errors.IsKind` 判定；校验失败仍可通过 `errors.Is` / `errors.As` 找到内层原因。
- `httpx.NewClient` 构建 transport 失败及重试拦截器发送失败时，外层错误改为 `*errors.Error`。
  内层原因仍可通过 `errors.Is` / `errors.As` 找到；依赖旧外层具体类型的调用方应改按错误链判定。
- 示例 `examples/customchain` 删除 `ErrAccountBanned` 变量，改用
  `errors.IsKind(err, errors.NewKind("customchain.account_banned"))` 判断封禁。该示例是 `main` 包；
  API 对比工具仍会将变量移除报告为不兼容变化。

### 变更

- 生产 Go 代码统一使用专用类型错误或 `errors.Error`，以 `Err` 保留内层原因；示例改用 `Kind` 判断业务错误。

### 新增

- `make check-errors` 扫描核心库、示例与独立 `agentguard` 子模块，并在提交钩子和 CI 拦截生产代码直接 `errors.New`、`fmt.Errorf` 与字符串 panic；测试夹具豁免。

## [v0.10.1] - 2026-09-28

### 新增

- `(*errors.Error).As(any) bool`：只响应包内遍历探针，其余目标返回 false，不改变标准 `errors.As` 语义；
  供 `KindOf` / `IsKind` / `AttrsOf` 借 `errors.As` 遍历整棵错误树。

### 修复

- `errors.KindOf` / `IsKind` / `AttrsOf` 遍历整棵错误树：原实现只沿 `*Error.Err` 单链向内走，
  `fmt.Errorf` 多个 `%w` 或 `errors.Join` 的后续分支上的 `*Error` 会被漏判（例：`fmt.Errorf("%w: %w", a, b)` 判不到 `b` 的分类）。
  现按前序深度优先遍历（同 `errors.Is` / `errors.As`），`KindOf` 取前序第一个非 nil 分类。v0.10.0 用户请直接升级。

## [v0.10.0] - 2026-09-27

### 新增

- `errors.Error{Op, Kind, Attrs, Err}`：统一的结构化错误类型。`Op` 记录发生在哪一步（取代 `fmt.Errorf` 文本前缀），
  `Kind` 记录分类，`Attrs`（`[]slog.Attr`）记录结构化附加信息，`Err` 经 `Unwrap` 保留内层错误；
  `Error()` 输出 `op: kind: k=v: 内层`，空段跳过，nil 接收者可用。构造直接写字面量。
- `errors.Kind` 接口（`Name() string`）与默认实现 `errors.NewKind(name)`：写法同 `logger.Event`，
  各系统维护自己的分类常量；默认实现可比较、可作 map 键。
- `errors.KindOf(err)`（从外到内第一个非 nil 分类）、`errors.IsKind(err, k)`（链上任意一层命中）、
  `errors.AttrsOf(err)`（从外到内合并附加信息）：穿过 `%w`、`errors.Join`、`RetryableError` 等任意包装；
  不可比较的 Kind 实现判为不相等，不 panic。

## [v0.9.0] - 2026-09-27

### 新增

- `Logger.InfoEvent(e, attrs...)` / `Logger.WarnEvent(e, attrs...)`：具名 Logger 的事件日志，语义同包级 `InfoEvent` / `WarnEvent`
  （`Name` 只求值一次、msg 与 event 同值、不改调用方 attrs），另带 `module` 固定字段；nil 接收者与 nil 事件可用。

## [v0.8.0] - 2026-09-26

> tag `v0.8.0` 指向 `3924fc6`；本段定版在 tag 之后补提交，故 tag 内的 CHANGELOG 仍把这些条目记在 `[Unreleased]` 下。

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
