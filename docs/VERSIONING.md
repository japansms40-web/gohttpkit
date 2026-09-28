# 版本策略

导出符号即契约。破坏性变更升 major；**0.x 阶段升 minor**（`0.2.0` → `0.3.0` 即允许断兼容）。新增导出符号至少升 minor（见下）；纯内部替换、修 bug 升 patch。

依赖：`go.mod` 不因治理类重构变动。打 tag 用 `vMAJOR.MINOR.PATCH`（如 `v0.3.0`）。

- **新增导出符号至少升 minor**，不走 patch（下游 `go get -u=patch` 预期不会出现新 API）。有疑问取更高一档。
- **弃用周期**：删除 / 改变导出符号前，先以 `// Deprecated: 用 Xxx 替代。将在 vX.Y.0 移除。` 标记并至少保留一个 minor。
  规则见 [`CODE_STANDARDS.md`](CODE_STANDARDS.md) §13。
- **上调 `go.mod` 的 `go` 指令**按 minor 处理。
- 每个版本的用户可见变化同时记入 [`../CHANGELOG.md`](../CHANGELOG.md)；打 tag、hotfix、`retract` 流程见 [`RELEASE.md`](RELEASE.md)。

## v0.15.0（相对 v0.14.0）

新增导出 API（0.x 阶段按 minor），下游直接升级：

- 新增 `logger.DebugEvent` / `logger.ErrorEvent` 与 `(*logger.Logger).DebugEvent` / `ErrorEvent`，事件日志四级齐全；
  语义与 `InfoEvent` / `WarnEvent` 一致（msg 与 event 同值）。已有调用不受影响。
- 新增 `httpx.EventHTTPBodyClose`（`http.body_close_failed`）。解压层关闭响应体失败的日志由自由文案
  `msg="failed to close response body"` 改为该事件：级别仍为 error，`error` 字段不变。下游若按旧 msg 文案过滤，改为按 `event` 过滤。
- `tools/agentguard/v0.1.6`：`check-pkg-doc` 不再把紧贴 `package`、只含指令的注释（如文件级 `//nolint:goconst // 理由`）
  当成包注释。下游接入 `check-pkg-doc` 时，这类抓包数据文件无需改动。

## v0.14.0（相对 v0.13.0）

破坏点（0.x 阶段按 minor）：

- 删除 `geo.MobileLocaleForCountry`（及 Android 下划线 locale 表），未经弃用周期（维护者决定）。
  迁移：Android locale 取值改用子包 `localemobile` 的五个 `XxxForCountry`；需要无 script 的 `zh_CN` 形态时，
  由调用方从 `localemobile.DeviceLocaleForCountry` 的结果裁剪。
- 新增 `geo.LookupCountry(table, country)`：按本包统一语义（trim + 大写、空或未命中返回 `*geo.UnknownCountryError`）查任意国家表。
- 各包包注释迁入 `doc.go` 并补文件结构树，`go doc` 只多出文件树。`tools/agentguard/v0.1.5` 新增 `check-pkg-doc`；
  下游升级 agentguard 不会自动启用，在自己的 Makefile / 钩子里接入前，先按 `CODE_STANDARDS.md` §13 整改。

## v0.13.0（相对 v0.12.0）

无导出 API 变化，下游直接升级。

- 仓库内测试文件按「一源一测」归并，并补齐缺失的测试。
- `tools/agentguard/v0.1.4` 新增 `check-test-layout`（同版本首次带上 `check-errors` 及其 Op / Kind 规则）。
  下游升级 agentguard 不会自动启用这两项检查；在自己的 Makefile / 钩子里接入前，先按 `CODE_STANDARDS.md` §5.1、§8 整改。

## v0.12.0（相对 v0.11.0）

新增与行为变更（0.x 阶段按 minor）：

- 新增导出 `versionreg.KindRegisterInvalid` / `KindRegisterEmptyID` / `KindRegisterDuplicate`，名称与 v0.11.0 的分类相同。
  下游把 `errors.IsKind(err, errors.NewKind("versionreg.register.duplicate"))` 换成 `errors.IsKind(err, versionreg.KindRegisterDuplicate)` 即可，不换也照常工作。
- 外层 `*errors.Error` 的 `Op` 改为 `<包>.<步骤>`：`"httpx: build transport"` → `"httpx.build_transport"`，
  `"failed to send request"` → `"interceptor.send_request"`，`Error()` 前缀随之变化。错误类型与错误链不变；
  下游若按文案前缀匹配，改为 `errors.As` / `errors.Is`。
- `tools/agentguard` 的 `check-errors` 新增 Op / Kind 规则（见 `CODE_STANDARDS.md` §5.1）。下游升级到 `tools/agentguard/v0.1.4`
  前，先把 `errors.Error` 字面量里的字符串 Op 改为具名 const、把函数体内的 `NewKind` 提到包级 var。

## v0.11.0（相对 v0.10.1）

破坏点与行为变更（0.x 阶段按 minor）：

- `versionreg.Registry.MustRegister` 的失败 panic 值统一为 `*errors.Error`，带 `Kind` 与注册表、版本字段。
  下游若对空标识或重复注册使用 `recover().(string)`，须改为恢复 `error` 后用 `errors.As` /
  `errors.IsKind` 判定；校验错误仍通过 `Unwrap` 保留原始原因。
- `httpx.NewClient` 构建 transport 失败及重试拦截器发送失败时，外层包装改为 `*errors.Error`；
  内层错误链保留。下游应按 `errors.As` / `errors.Is` 判定原因，不依赖旧外层具体类型。
- `examples/customchain.ErrAccountBanned` 被移除，示例改用
  `errors.IsKind(err, errors.NewKind("customchain.account_banned"))`；`apidiff` 将该 `main` 包变量移除报告为不兼容变化。

## v0.10.1（相对 v0.10.0）

修复（不破坏现有签名）：

- `errors.KindOf` / `IsKind` / `AttrsOf` 改为遍历整棵错误树，修复多个 `%w` / `errors.Join` 分支漏判；新增 `(*errors.Error).As`。
  下游无需迁移，直接升级。

## v0.10.0（相对 v0.9.0）

新增（不破坏现有签名）：

- `errors.Error`、`errors.Kind`、`errors.NewKind`、`errors.KindOf`、`errors.IsKind`、`errors.AttrsOf`：结构化错误与分类接口。
  现有错误类型与判定函数不变，下游无需迁移；新代码建议统一返回 `&errors.Error{Op, Kind, Attrs, Err}`。

## v0.9.0（相对 v0.8.0）

新增（不破坏现有签名）：

- `Logger.InfoEvent` / `Logger.WarnEvent`：具名 Logger 的事件日志，语义同包级 `InfoEvent` / `WarnEvent`，另带 `module` 字段。
  既有方法签名与行为不变，下游无需迁移。

## v0.8.0（相对 v0.7.0）

新增（不破坏现有签名）：

- `logger.Named`、`logger.Logger`（`With` / `Debug` / `Info` / `Warn` / `Error`）、`logger.FieldModule`：不需要 ctx 的具名日志句柄。
  `logger.Info(ctx, ...)` 等 ctx 版门面签名与行为不变，下游无需迁移；拿不到 ctx 的调用点可改用 `Named`，但它不带 trace_id。

## v0.7.0（相对 v0.6.0）

行为变更（签名不变，0.x 阶段按 minor）：

- 重试拦截器：调用方 ctx 结束引起的发送失败不再重试，返回 `failed to send request: %w` 而非 `*errors.RetryableError`。
  下游若用 `errors.As(err, &RetryableError)` 决定「换出口重试」，调用方自己的超时 / 取消将不再命中该分支——
  这正是修复目的；需要识别超时请改用 `errors.Is(err, context.DeadlineExceeded)` / `errors.Is(err, context.Canceled)`。

## v0.6.0（相对 v0.5.0）

新增（不破坏现有签名）：

- `httpx.StatusRuleContext`、`interceptor.NewStatusSemanticsInterceptorContext`：状态语义规则额外拿到本次请求的 ctx。
  `NewStatusSemanticsInterceptor` 签名与行为不变，下游无需迁移。

## v0.5.0（相对 v0.4.1）

破坏点：

- 删除 `interceptor.NewClient`，入口改为 `httpx.NewClient`（未传 Interceptors 时装由 `httpx/interceptor` 注册的默认链）。
  按维护者决定**未经弃用期直接删除**（不同于上面的弃用周期规则）；下游把 `interceptor.NewClient(` 替换为 `httpx.NewClient(`，
  并确认程序里仍 import 了 `httpx/interceptor`（否则得到 `*httpx.NoDefaultChainError`）。

新增（不破坏现有签名）：

- `httpx.NewClient`、`httpx.RegisterDefaultChain`、`httpx.NoDefaultChainError`。

## v0.4.0（相对 v0.3.3）

破坏点：

- `go.mod` 的 `go` 指令从 1.25.0 上调到 1.27.1（`tools/agentguard` 从 1.24.0 上调到 1.27.1）。下游须使用 Go 1.27.1 及以上才能编译本库。此后有新的稳定版继续上调，CI 用 `stable`，不钉死旧版本。

安全：

- `golang.org/x/net` 从 v0.55.0 升到 v0.59.0，修复经 `html.Parse` 可达的 HTML 解析漏洞，以及 HTTP/2 相关漏洞。直接依赖保持最新稳定版，不钉死旧版本。

## v0.3.1（相对 v0.3.0）

新增（不破坏现有签名）：

- 子包 `geo/locale_mobile`（package `localemobile`）：`AcceptLanguageForCountry`、`AppLocaleForCountry`、
  `DeviceLocaleForCountry`、`MappedLocaleForCountry`、`DeviceLanguagesForCountry`，未命中返回 `*geo.UnknownCountryError`。

## v0.3.0（相对 v0.2.0）

破坏点：

- `httpx.ContentEncodingError.Encoding`、`httpx.ReadResponseBodyError.Encoding` 由 `string` 改为 `httpx.ContentEncoding`。
  下游若按 `string` 直接读该字段，需加 `string(...)` 或与 `httpx.EncodingGzip` 等枚举比较。

新增（不破坏现有签名）：

- `httpx.ContentEncoding` / `ParseContentEncoding`
- `httpx.ReadResponseBodyError.RawEncoding`（新增字段，保留未识别 content-encoding 原文；不破坏 `errors.As` 用法）
- `httpx.Header*` 全套 header 名 const（含 `HeaderUserAgentCanonical`）
- `httpx.LogField*`、`logger.Field*` 日志字段 key const
- `httpx.StatusClass` / `ClassifyStatus` / `IsSuccessStatus` / `IsErrorStatus`
