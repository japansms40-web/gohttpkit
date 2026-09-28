// Package geo 提供「地理派生」基建：国家代码 → Web locale / 时区偏移，
// 以及按 Chromium 规则拼 Web Accept-Language（不含具体 App 的业务 header 名）。
//
// 典型用途：走住宅代理出网时，请求头里的 Accept-Language、时区、locale 必须与代理出口
// 国家一致，否则服务端一眼看出「IP 在印尼、语言是简体中文」的画像矛盾。本包把这套派生
// 收敛成一组纯函数，供 httpx.HeaderProvider 的实现调用。
//
// 设计要点：
//   - 一表一文件：每张「国家码 → 值」字面量表独立成文件，配对应的 XxxForCountry（逐文件见下方文件结构）。
//     web locale 与时区偏移两表 keyset 由 TestTimezoneTableKeysetMatchesLocaleTable 守护。
//   - WebAcceptLanguageForCountry 只返回 Chrome data-code（如 zh-CN / ja）。完整
//     Accept-Language 由 BuildChromeAcceptLanguage / BuildChromeAcceptLanguageForCountry
//     按 Chromium 前瞻展开和 q 值规则构建。调用方传入的 *rand.Rand 不能被多个
//     goroutine 共享，若共享须自行加锁；rng == nil 时用 math/rand/v2 顶层源，可并发。
//   - Android 端 locale 只在子包 geo/locale_mobile（包名 localemobile）维护：Meta 系 App 五个 locale header
//     一个 header 一个文件，一张字面量表 + 一个 XxxForCountry，keyset 与本包对齐。本包不再另存 Android locale 表。
//   - 查表公用函数在 lookup.go；LookupCountry 导出给子包与下游自建国家表复用，保证归一化与未命中错误一致。
//     显式 locale / country 的优先级由调用方组合。
//   - 查表未命中是 *UnknownCountryError；extra 负数是 *InvalidExtraLanguageCountError；
//     extra 超出候选是 *ExtraLanguageCountExceedsPoolError。上层一律 errors.As，不要扫文案。
//   - 纯函数 + 表驱动，不引入「为模式而模式」的接口/抽象。
//
// 本包零外部依赖（仅标准库），可独立使用与测试。
//
// 文件结构：
//
//	geo/
//	├── doc.go            包文档（本文件）
//	├── errors.go         本包类型错误：UnknownCountryError 等
//	├── locale_web.go     国家 → Chrome data-code，及 Chromium 规则的 Accept-Language 拼装
//	├── lookup.go         LookupCountry：国家码归一化查表与未命中错误（子包共用）
//	├── timezone.go       国家 → 冬令时偏移秒数（TimezoneOffsetForCountry）
//	├── timezone_iana.go  国家 → IANA 时区名称（子集表）
//	└── locale_mobile/    子包 localemobile：Android 端五个 locale header
package geo
