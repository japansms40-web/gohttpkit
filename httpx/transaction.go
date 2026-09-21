package httpx

// Transaction 一次请求/响应的完整快照（含请求头与体、响应头与体），供调试落盘。
// json tag 与 log_fields.go 的 LogField* 同值；struct tag 不能引用 const，改字段名时两处一起改。
//
// cookie / authorization 原样保留，用途是与真实抓包逐字段对比。
// 别把它写到会被共享或长期留存的地方。
type Transaction struct {
	// Method HTTP 方法。
	Method string `json:"method"`
	// URL 实际请求的完整 URL。
	URL string `json:"url"`
	// Proxy 本次使用的代理 URL，直连时为空。
	Proxy string `json:"proxy"`
	// ExitIP 调用方传入的出口公网 IP，本库不探测。空表示未提供。
	ExitIP string `json:"exit_ip"`
	// ASN 调用方传入的出口 ASN，本库不探测。空表示未提供。
	ASN string `json:"asn"`
	// ReqHeaders 第一次尝试发出的请求头快照（含 cookie / authorization）。
	ReqHeaders map[string][]string `json:"req_headers"`
	// ReqBody 请求体原文。
	ReqBody string `json:"req_body"`
	// ReqBodyLen 请求体原始字节数（不受日志截断影响）。
	ReqBodyLen int `json:"req_body_len"`
	// Status 响应状态码。
	Status int `json:"status"`
	// RespHeaders 响应头（可能含 set-cookie）。
	RespHeaders map[string][]string `json:"resp_headers"`
	// RespBody 最终对外的响应体（已经过链上可能的提纯/改写）。
	RespBody string `json:"resp_body"`
	// RespBodyLen 响应体原始字节数。
	RespBodyLen int `json:"resp_body_len"`
	// DurationMS 端到端耗时（本层在链最外，含重试与解压）。
	DurationMS int64 `json:"duration_ms"`
}
