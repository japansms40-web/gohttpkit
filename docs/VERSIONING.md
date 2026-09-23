# 版本策略

导出符号即契约。破坏性变更升 major；**0.x 阶段升 minor**（`0.2.0` → `0.3.0` 即允许断兼容）。新增导出符号或纯内部替换升 patch / minor，按影响面判断。

依赖：`go.mod` 不因治理类重构变动。打 tag 用 `vMAJOR.MINOR.PATCH`（如 `v0.3.0`）。

## v0.4.0（未发布，相对 v0.3.0）

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
