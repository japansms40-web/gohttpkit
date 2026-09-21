package httpx

// log_fields.go —— HTTP 交易 / 重试日志字段 key。单一事实源，禁止在拦截器打点处再写同名字面量。
// Transaction 的 json tag 必须与下列常量同值（struct tag 不能引用 const）。
// 不作 type 枚举：它们只当 slog.Attr 的 key。

// HTTP 交易 / 重试日志字段 key，与 Transaction 的 json tag 同值。
const (
	LogFieldMethod      = "method"
	LogFieldURL         = "url"
	LogFieldProxy       = "proxy"
	LogFieldExitIP      = "exit_ip"
	LogFieldASN         = "asn"
	LogFieldReqHeaders  = "req_headers"
	LogFieldReqParams   = "req_params"
	LogFieldReqBody     = "req_body"
	LogFieldReqBodyLen  = "req_body_len"
	LogFieldStatus      = "status"
	LogFieldProto       = "proto"
	LogFieldProtoMajor  = "proto_major"
	LogFieldRespHeaders = "resp_headers"
	LogFieldRespBody    = "resp_body"
	LogFieldRespBodyLen = "resp_body_len"
	LogFieldDurationMS  = "duration_ms"
	LogFieldSlow        = "slow"

	LogFieldAttempt    = "attempt"
	LogFieldMaxRetries = "max_retries"
	LogFieldBackoff    = "backoff"
	LogFieldErrType    = "err_type"
)
