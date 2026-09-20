// Package geo 提供「地理派生」基建：国家代码 → locale / 时区偏移、代理 URL → 出口国家、
// 以及按 Chromium 规则拼 Web Accept-Language（不含具体 App 的业务 header 名）。
//
// 典型用途：走住宅代理出网时，请求头里的 Accept-Language、时区、locale 必须与代理出口
// 国家一致，否则服务端一眼看出「IP 在印尼、语言是简体中文」的画像矛盾。本包把这套派生
// 收敛成一组纯函数，供 httpx.HeaderProvider 的实现调用。
//
// 设计要点：
//   - 各表各函数：locale_web.go（countryToWebAcceptTag + WebAcceptLanguageForCountry）、
//     locale_mobile.go（countryToMobileLocale + MobileLocaleForCountry）、
//     timezone.go（countryToTimezoneOffset + TimezoneOffsetForCountry）。三表 keyset 由
//     TestTimezoneTableKeysetMatchesLocaleTable 守护。
//   - WebAcceptLanguageForCountry 只返回 Chrome data-code（如 zh-CN / ja）。完整
//     Accept-Language 由 BuildChromeAcceptLanguage / BuildChromeAcceptLanguageForCountry
//     按 Chromium 前瞻展开和 q 值规则构建。调用方传入的 *rand.Rand 不能被多个
//     goroutine 共享，若共享须自行加锁；rng == nil 时用 math/rand/v2 顶层源，可并发。
//   - 查表公用函数在 lookup.go。显式 locale / country / proxy 的优先级由调用方组合。
//   - 查表未命中是 *UnknownCountryError；extra 负数是 *InvalidExtraLanguageCountError；
//     extra 超出候选是 *ExtraLanguageCountExceedsPoolError。上层一律 errors.As，不要扫文案。
//   - 纯函数 + 表驱动，不引入「为模式而模式」的接口/抽象。
//
// 本包零外部依赖（仅标准库），可独立使用与测试。
package geo
