package httpx

import "net/http"

// status_class.go —— HTTP 状态码按百位归类。禁止在拦截器里再写 200/300/400 裸区间。

// StatusClass 按百位归类的 HTTP 状态语义。
// 给 statusSemantics / logging 与自定义拦截器判断 2xx / 4xx / 5xx。
type StatusClass uint8

const (
	ClassUnknown StatusClass = iota
	ClassInformational
	ClassSuccess
	ClassRedirection
	ClassClientError
	ClassServerError
)

// ClassifyStatus 按百位归类状态码。
// 输入 code：任意整数；边界用 net/http 常量（http.StatusOK 等）。
// 返回：1xx–5xx 对应 Class*；其余（含负数、0、600+）为 ClassUnknown。
// 例：ClassifyStatus(http.StatusOK) → ClassSuccess；ClassifyStatus(600) → ClassUnknown。
func ClassifyStatus(code int) StatusClass {
	switch {
	case code >= http.StatusContinue && code < http.StatusOK:
		return ClassInformational
	case code >= http.StatusOK && code < http.StatusMultipleChoices:
		return ClassSuccess
	case code >= http.StatusMultipleChoices && code < http.StatusBadRequest:
		return ClassRedirection
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return ClassClientError
	case code >= http.StatusInternalServerError && code < http.StatusInternalServerError+100:
		return ClassServerError
	default:
		return ClassUnknown
	}
}

// IsSuccessStatus 判定是否 2xx。
// 输入 code：任意整数。
// 返回：ClassifyStatus(code) == ClassSuccess。
// 例：IsSuccessStatus(http.StatusOK) → true；IsSuccessStatus(http.StatusBadRequest) → false。
func IsSuccessStatus(code int) bool { return ClassifyStatus(code) == ClassSuccess }

// IsErrorStatus 判定是否应视为出错状态（原 logging 的 >= 400 语义）。
// 输入 code：任意整数。
// 返回：4xx / 5xx 为 true；600+ 仍为 true，以保持旧的「>= 400 打全量日志」行为。
// 例：IsErrorStatus(http.StatusBadRequest) → true；IsErrorStatus(http.StatusOK) → false。
func IsErrorStatus(code int) bool {
	switch ClassifyStatus(code) {
	case ClassClientError, ClassServerError:
		return true
	default:
		return code >= http.StatusBadRequest
	}
}
