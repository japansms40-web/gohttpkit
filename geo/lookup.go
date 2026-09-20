package geo

import "strings"

// lookup.go —— 国家码查表公用函数。Web / Android / 时区各自一表一函数，归一化与未命中错误在这里共用。

// normalizeCountryKey 把任意国家码收成查表 key。
// 输入 country：原始国家码，可带空白、大小写混用。
// 返回：trim 后的大写串；全空白返回 ""。
func normalizeCountryKey(country string) string {
	return strings.ToUpper(strings.TrimSpace(country))
}

// lookupCountry 在给定静态表里查国家码。
// 输入 table：国家码 → 值的只读表（不会改）；country：原始国家码（内部先归一化）。
// 返回：命中是表值；空输入或未命中是 ("", *UnknownCountryError)，Country 为归一化码（空输入为 ""）。
func lookupCountry(table map[string]string, country string) (string, error) {
	cc := normalizeCountryKey(country)
	if cc == "" {
		return "", &UnknownCountryError{Country: ""}
	}
	v, ok := table[cc]
	if !ok {
		return "", &UnknownCountryError{Country: cc}
	}
	return v, nil
}
