package main

import kiterrors "github.com/japansms40-web/gohttpkit/errors"

// errors.go —— 本示例的错误定义（Op / Kind / Attrs），写法见 docs/CODE_STANDARDS.md §5.1。

const (
	opParseURL  = "quickstart.parse_url"
	opNewClient = "quickstart.new_client"
	opGet       = "quickstart.get"
)

// reasonAttrKey 说明失败原因的 Attr key；原因是诊断信息，不塞进 Op。
const (
	reasonAttrKey             = "reason"
	reasonMissingSchemeOrHost = "missing_scheme_or_host"
)

// kindInvalidURL -url 参数不是完整 URL。
var kindInvalidURL = kiterrors.NewKind("quickstart.invalid_url")
