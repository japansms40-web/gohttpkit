package httpx

import "strings"

// encoding.go —— 本库能解压的响应体编码。解析 content-encoding 后只拿枚举做分支，
// 禁止在 bodyDecode / 解压错误构造里再写 "zstd" / "gzip" / "deflate" / "br" 字面量。

// ContentEncoding 本库能解压的响应体编码。
// 给 bodyDecode 分支与 ContentEncodingError / ReadResponseBodyError 字段；
// 未登记的编码不要硬转成本类型。
type ContentEncoding string

const (
	// EncodingIdentity 未压缩（缺头或空值）。
	EncodingIdentity ContentEncoding = ""
	// EncodingZstd zstd 压缩。
	EncodingZstd ContentEncoding = "zstd"
	// EncodingGzip gzip 压缩。
	EncodingGzip ContentEncoding = "gzip"
	// EncodingDeflate raw deflate 压缩。
	EncodingDeflate ContentEncoding = "deflate"
	// EncodingBr brotli 压缩。
	EncodingBr ContentEncoding = "br"
)

// String 实现 fmt.Stringer。
// 输入：接收者是枚举本身。
// 返回：content-encoding 头里用的小写值；零值 / EncodingIdentity 是 ""。
func (e ContentEncoding) String() string { return string(e) }

// ParseContentEncoding 把 content-encoding 头收成枚举。
// 输入 raw：响应头原文，大小写不敏感，不做 trim。
// 返回：zstd / gzip / deflate / br → (对应枚举, true)；空或未登记 → (EncodingIdentity, false)。
// 例：ParseContentEncoding("GZIP") → (EncodingGzip, true)；ParseContentEncoding("identity") → ("", false)。
func ParseContentEncoding(raw string) (ContentEncoding, bool) {
	switch ContentEncoding(strings.ToLower(raw)) {
	case EncodingZstd:
		return EncodingZstd, true
	case EncodingGzip:
		return EncodingGzip, true
	case EncodingDeflate:
		return EncodingDeflate, true
	case EncodingBr:
		return EncodingBr, true
	default:
		return EncodingIdentity, false
	}
}
