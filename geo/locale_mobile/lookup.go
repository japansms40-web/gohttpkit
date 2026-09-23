package localemobile

import (
	"strings"

	"github.com/japansms40-web/gohttpkit/geo"
)

// lookup.go —— 国家码查表公用函数；归一化与未命中错误和 geo 包保持一致。

// lookupCountry 在给定静态表里查国家码。
// 输入 table：国家码 → 值的只读表（不会改）；country：原始国家码，trim 后转大写。
// 返回：命中是表值；空输入或未命中是 ("", *geo.UnknownCountryError)，Country 为归一化码（空输入为 ""）。
func lookupCountry(table map[string]string, country string) (string, error) {
	cc := strings.ToUpper(strings.TrimSpace(country))
	if cc == "" {
		return "", &geo.UnknownCountryError{Country: ""}
	}
	v, ok := table[cc]
	if !ok {
		return "", &geo.UnknownCountryError{Country: cc}
	}
	return v, nil
}
