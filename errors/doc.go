// Package errors 提供与业务无关的错误基建：结构化错误 Error + 分类接口 Kind、
// 可重试错误包装、瞬时网络错误判定、HTTP 状态码错误。
//
// 结构化错误（error.go / kind.go）：接入方统一返回 &errors.Error{Op, Kind, Attrs, Err}，
// 不用 fmt.Errorf 堆文本前缀；各系统用 NewKind（或自行实现 Kind 接口）维护自己的分类，
// 判定用 KindOf / IsKind，读字段用 errors.AsType[*errors.Error]。
//
// 定义放在产生错误的包的 errors.go（本包只放跨包基建）：Op 为未导出 const，值 "<包>.<步骤>"；
// Kind 为导出包级 var，值 "<包>.<分类>"，名称即契约。细则见 docs/CODE_STANDARDS.md §5.1。
//
//	const opGetUser = "svc.get_user"
//	var KindLoginRequired = errors.NewKind("insgo.login_required")
//	return nil, &errors.Error{Op: opGetUser, Kind: KindLoginRequired, Err: err}
//	if errors.IsKind(err, KindLoginRequired) { ... }
//
// 网络错误判定用「错误文案关键词表」而非 net.Error.Temporary()——经过 SOCKS5 代理、
// TLS、HTTP/2 多层包装后，底层错误类型早已丢失，只有文案还留着线索。关键词表可经
// RegisterRetryableKeywords 扩展，也可经 httpx.RetryPolicy.IsRetryable 整体替换。
//
// 测试专用快照见 export_test.go（仅 go test 编译，不进生产 API）。
//
// 文件结构：
//
//	errors/
//	├── doc.go     包文档（本文件）
//	├── error.go   结构化错误 Error{Op, Kind, Attrs, Err} 与 KindOf / IsKind / AttrsOf
//	├── errors.go  RetryableError、瞬时网络错误关键词判定、HTTPStatusError
//	└── kind.go    分类接口 Kind 与 NewKind
package errors
