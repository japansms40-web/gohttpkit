// Package localemobile 按国家码给出 Android 端（Meta 系 App）五个 locale 相关 header 的值：
// Accept-Language、X-IG-App-Locale、X-IG-Device-Locale、X-IG-Mapped-Locale、X-IG-Device-Languages。
//
// 每个 header 一个文件：一张「国家码 → 值」的字面量表 + 一个 XxxForCountry 查表函数，写法同
// geo/locale_web.go。取值规则来自 IG Android 429.1.0.44.70 的字节码（各文件表头写明方法），
// 值预先算好写进字面量，运行期不做推导。
//
// 画像假设：系统语言列表只有该国一项（取 AOSP supported_locales 中该国主语言的可选项，带 script）、
// 未在 App 内单独设语言、键盘语言与系统语言相同、设备语言 header 的门控打开。真机不满足这些
// 假设时（多语言列表、App 内切语言、外语键盘），值会不同。
//
// 五张表 keyset 相同，并与 geo 的国家表对齐，由测试守护。
// 空输入或未命中返回 *geo.UnknownCountryError，上层一律 errors.As，不要扫文案。
// 目录名 locale_mobile 避开 _android 后缀（Go 会把它当成 GOOS 构建约束）。
package localemobile
