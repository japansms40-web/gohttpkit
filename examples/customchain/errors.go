package main

import kiterrors "github.com/japansms40-web/gohttpkit/errors"

// errors.go —— 本示例的错误定义（Op / Kind / Attrs），写法见 docs/CODE_STANDARDS.md §5.1。

const (
	opClassify  = "customchain.classify"
	codeAttrKey = "code" // 业务响应里的 code 字段
)

// kindAccountBanned 业务层判定的账号封禁（HTTP 200 + status=fail/code=banned）。
var kindAccountBanned = kiterrors.NewKind("customchain.account_banned")
