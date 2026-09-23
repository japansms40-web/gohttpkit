# 更新日志

格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [`docs/VERSIONING.md`](docs/VERSIONING.md)，
发布流程见 [`docs/RELEASE.md`](docs/RELEASE.md)。每个版本按「破坏 / 新增 / 修复 / 弃用 / 安全」分组，空组省略。

## [Unreleased]

### 破坏

- 最低 Go 版本从 1.24 升到 1.25。使用本库的模块须升级到 Go 1.25 或更高才能编译。

### 新增

- 规范：`docs/RELEASE.md`、`SECURITY.md`、本文件；`CODE_STANDARDS` §10–16、`TESTING` §8–11；AI 代理硬性纪律。
- 门禁：`nolintlint`；CI 显式 `make char`；治理守卫 `make governance`（CI / pre-push / agent 收尾）；
  Claude / Cursor / Codex 三家 agent 钩子（`tools/agentguard`，仓库工具，不影响库的导出 API）。

### 安全

- 升级 `golang.org/x/net` 到 v0.55.0，修复经 `httpx.ExtractHTMLText` → `html.Parse` 可达的 HTML 解析漏洞，以及 HTTP/2 相关漏洞。

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

### 新增

- 全包测试覆盖到 98.1%，加覆盖率门禁。

## [v0.1.0] - 2026-08-25

### 新增

- 初版：`httpx`、`geo`、`logger`、`netproxy`、`traffic`、`versionreg`、`errors`。
