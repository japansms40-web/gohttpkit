// Package geo 提供「地理派生」基建：国家代码 → locale / 时区偏移、代理 URL → 出口国家、
// 以及移动端/Web 端各 header 的 locale 格式派生。
//
// 典型用途：走住宅代理出网时，请求头里的 Accept-Language、时区、locale 必须与代理出口
// 国家一致，否则服务端一眼看出「IP 在印尼、语言是简体中文」的画像矛盾。本包把这套派生
// 收敛成一组纯函数，供 httpx.HeaderProvider 的实现调用。
//
// 设计要点：
//   - 大数据映射表（国家→locale、国家→时区）独立成文件，表头注明数据源与维护规约：
//     locale.go（countryToWebLocale / countryToMobileLocale）、timezone.go
//     （countryToTimezoneOffset）；两表 keyset 由 TestTimezoneTableKeysetMatchesLocaleTable 守护。
//   - 本包只提供「无副作用的纯派生函数」，优先级链（显式 locale > CountryCode > 代理出口国）
//     由调用方组合，不在三处各算各的。
//   - 纯函数 + 表驱动，不引入「为模式而模式」的接口/抽象。
//
// 本包零外部依赖（仅标准库），可独立使用与测试。
package geo
