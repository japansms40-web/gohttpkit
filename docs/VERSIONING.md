# 版本策略

导出符号即契约。破坏性变更升 major；**0.x 阶段升 minor**（`0.2.0` → `0.3.0` 即允许断兼容）。新增导出符号至少升 minor（见下）；纯内部替换、修 bug 升 patch。

依赖：`go.mod` 不因治理类重构变动。打 tag 用 `vMAJOR.MINOR.PATCH`（如 `v0.3.0`）。

- **新增导出符号至少升 minor**，不走 patch（下游 `go get -u=patch` 预期不会出现新 API）。有疑问取更高一档。
- **弃用周期**：删除 / 改变导出符号前，先以 `// Deprecated: 用 Xxx 替代。将在 vX.Y.0 移除。` 标记并至少保留一个 minor。
  规则见 [`CODE_STANDARDS.md`](CODE_STANDARDS.md) §13。
- **上调 `go.mod` 的 `go` 指令**按 minor 处理。
- 每个版本的用户可见变化同时记入 [`../CHANGELOG.md`](../CHANGELOG.md)；打 tag、hotfix、`retract` 流程见 [`RELEASE.md`](RELEASE.md)。

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
