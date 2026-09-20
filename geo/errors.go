package geo

import "fmt"

// InvalidExtraLanguageCountError extra 为负数。
// Extra 是调用方传入的非法值，便于上层直接读，不必扫文案。
// 判定请用 errors.As。
type InvalidExtraLanguageCountError struct {
	// Extra 调用方传入的 extra；由本包返回时恒 < 0。
	Extra int
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "geo: extra language count is invalid <nil>"；
// 否则 → `geo: extra language count must be non-negative: extra=-1`。
func (e *InvalidExtraLanguageCountError) Error() string {
	if e == nil {
		return "geo: extra language count is invalid <nil>"
	}
	return fmt.Sprintf("geo: extra language count must be non-negative: extra=%d", e.Extra)
}

// ExtraLanguageCountExceedsPoolError extra 大于去重排序后的剩余候选数。
// Extra / Available 便于上层决定是缩小 extra 还是放弃随机。判定请用 errors.As。
type ExtraLanguageCountExceedsPoolError struct {
	// Extra 调用方要求的额外项数。
	Extra int
	// Available 排除主码展开项后的剩余候选数。
	Available int
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "geo: extra language count exceeds candidate pool <nil>"；
// 否则 → `geo: extra language count exceeds candidate pool: extra=10 available=3`。
func (e *ExtraLanguageCountExceedsPoolError) Error() string {
	if e == nil {
		return "geo: extra language count exceeds candidate pool <nil>"
	}
	return fmt.Sprintf("geo: extra language count exceeds candidate pool: extra=%d available=%d", e.Extra, e.Available)
}

// UnknownCountryError 国家码为空，或不在 countryToWebAcceptTag / countryToMobileLocale
// （及对齐的 timezone offset 表）。
// 判定请用 errors.As，不要扫 Error() 文案；Country 是归一化后的 ISO 3166-1 alpha-2，
// 空输入时为 ""，便于上层区分「没给国家」和「给了但不在表里」。
type UnknownCountryError struct {
	// Country 归一化后的国家码；空输入或 trim 后为空时是 ""。
	Country string
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "geo: unknown country <nil>"；Country=="" → "geo: country code is empty"；
// 否则 → `geo: unknown country "XX"`。
// 英文短句便于跨语言 grep；空码与未知码分开写，上层不要再解析文案。
func (e *UnknownCountryError) Error() string {
	if e == nil {
		return "geo: unknown country <nil>"
	}
	if e.Country == "" {
		return "geo: country code is empty"
	}
	return fmt.Sprintf("geo: unknown country %q", e.Country)
}
