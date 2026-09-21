package httpx

import "fmt"

// errors.go —— 本包身份错误。判定请用 errors.As 读字段，不要扫 Error() 文案。

// MissingHeaderProviderError New 时 Options.Headers 为 nil。
// Field 恒为 "Options.Headers"，便于上层直接读。判定请用 errors.As。
type MissingHeaderProviderError struct {
	// Field 缺失的 Options 字段名。
	Field string
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: missing header provider <nil>"；
// 否则 → 当前 New 的必填提示（文案保持兼容）。
func (e *MissingHeaderProviderError) Error() string {
	if e == nil {
		return "httpx: missing header provider <nil>"
	}
	return "httpx: Options.Headers is required (implement HeaderProvider; StaticHeaders is the simplest)"
}

// ChainExhaustedError 链游标越过末尾：组装时缺终端拦截器。
// Index / Length 是越界当时的游标与链长。判定请用 errors.As。
type ChainExhaustedError struct {
	// Index 当前游标（已 >= Length）。
	Index int
	// Length 当时链上的拦截器个数。
	Length int
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: interceptor chain exhausted <nil>"；
// 否则 → 缺终端的组装提示（文案保持兼容）。
func (e *ChainExhaustedError) Error() string {
	if e == nil {
		return "httpx: interceptor chain exhausted <nil>"
	}
	return "httpx: interceptor chain exhausted — missing terminal interceptor (e.g. NewCallServerInterceptor)"
}

// NilBuildHeadersError HeaderProvider.BuildHeaders 返回了 nil。
// 空 map 表示「没有头」；nil 表示实现出错。判定请用 errors.As。
type NilBuildHeadersError struct {
	// ProviderType 当时 Headers() 的 %T。
	ProviderType string
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: nil build headers <nil>"；
// 否则 → "httpx: HeaderProvider.BuildHeaders returned nil"。
func (e *NilBuildHeadersError) Error() string {
	if e == nil {
		return "httpx: nil build headers <nil>"
	}
	return "httpx: HeaderProvider.BuildHeaders returned nil"
}

// RequestBodyEncodeError json.Marshal 请求体失败。
// BodyType 是调用方传入值的 %T；Err 是 encoding/json 的底层错误。判定请用 errors.As。
type RequestBodyEncodeError struct {
	// BodyType 未能编码的值的类型名。
	BodyType string
	// Err json.Marshal 返回的错误。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: encode request body <nil>"；
// 否则 → `httpx: encode request body as JSON (T): 底层文案`。
func (e *RequestBodyEncodeError) Error() string {
	if e == nil {
		return "httpx: encode request body <nil>"
	}
	return fmt.Sprintf("httpx: encode request body as JSON (%s): %v", e.BodyType, e.Err)
}

// Unwrap 返回底层 json 错误。
// 输入：接收者或 Err 可为 nil。
// 返回：nil 接收者 / 空 Err → nil；否则 → Err。
func (e *RequestBodyEncodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ResponseJSONDecodeError json.Unmarshal 响应体失败。
// TargetType 是目标指针的 %T。判定请用 errors.As。
type ResponseJSONDecodeError struct {
	// TargetType 解码目标的类型名。
	TargetType string
	// Err json.Unmarshal 返回的错误。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: decode response JSON <nil>"；
// 否则 → `httpx: decode response JSON: 底层文案`（不含 TargetType，保持旧文案）。
func (e *ResponseJSONDecodeError) Error() string {
	if e == nil {
		return "httpx: decode response JSON <nil>"
	}
	return fmt.Sprintf("httpx: decode response JSON: %v", e.Err)
}

// Unwrap 返回底层 json 错误。
// 输入：接收者或 Err 可为 nil。
// 返回：nil 接收者 / 空 Err → nil；否则 → Err。
func (e *ResponseJSONDecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// CreateHTTPRequestError http.NewRequestWithContext 失败。
// 只保存 Method，不保存 URL，避免 query / userinfo 被 %+v 再复制一份。判定请用 errors.As。
type CreateHTTPRequestError struct {
	// Method 当时的 HTTP 方法。
	Method string
	// Err 标准库建请求失败。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: create http request <nil>"；
// 否则 → `failed to create request: 底层文案`。
func (e *CreateHTTPRequestError) Error() string {
	if e == nil {
		return "httpx: create http request <nil>"
	}
	return fmt.Sprintf("failed to create request: %v", e.Err)
}

// Unwrap 返回底层建请求错误。
// 输入：接收者或 Err 可为 nil。
// 返回：nil 接收者 / 空 Err → nil；否则 → Err。
func (e *CreateHTTPRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ContentEncodingError 按 content-encoding 构造解压 reader 失败。
// Encoding 为 EncodingGzip / EncodingZstd 等枚举。判定请用 errors.As。
type ContentEncodingError struct {
	// Encoding 失败的编码。
	Encoding ContentEncoding
	// Err reader 构造错误。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: content encoding <nil>"；
// 否则 → `failed to create {encoding} reader: 底层文案`。
func (e *ContentEncodingError) Error() string {
	if e == nil {
		return "httpx: content encoding <nil>"
	}
	return fmt.Sprintf("failed to create %s reader: %v", e.Encoding, e.Err)
}

// Unwrap 返回底层 reader 构造错误。
// 输入：接收者或 Err 可为 nil。
// 返回：nil 接收者 / 空 Err → nil；否则 → Err。
func (e *ContentEncodingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ReadResponseBodyError 读响应体失败且不是可重试网络错。
// Encoding 是当时的 ContentEncoding（未压缩时为 EncodingIdentity）。判定请用 errors.As。
type ReadResponseBodyError struct {
	// Encoding 当时的 content-encoding，未压缩时为 EncodingIdentity。
	Encoding ContentEncoding
	// Err io.ReadAll 的非重试错误。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "httpx: read response body <nil>"；
// 否则 → `failed to read response: 底层文案`。
func (e *ReadResponseBodyError) Error() string {
	if e == nil {
		return "httpx: read response body <nil>"
	}
	return fmt.Sprintf("failed to read response: %v", e.Err)
}

// Unwrap 返回底层读体错误。
// 输入：接收者或 Err 可为 nil。
// 返回：nil 接收者 / 空 Err → nil；否则 → Err。
func (e *ReadResponseBodyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// TransportError 标记「网络发送本身失败」的错误，只有它才会被 retry 拦截器纳入可重试判断；
// 其它错误（构建请求失败、构头失败等）原样穿透、立即返回。
//
// 自定义终端拦截器必须把 http.Client.Do 的错误包成 *TransportError，否则重试层看不见它。
//
// 约束：位于 retry 与终端之间的拦截器不得再包装（wrap）该错误 —— retry 用 errors.As 解包，
// 多包一层虽仍能识别，但那段区间语义上只应透传。
type TransportError struct {
	// Err 底层发送失败（dial / TLS / 被对端重置等）。retry 用 errors.As 解到本类型后再看它。
	Err error
}

// Error 透传底层文案，不另加前缀：接入方与关键词表匹配的是对端/代理的原始句子。
// 输入：接收者可为 nil；Err 也可为 nil。
// 返回：正常失败是底层 Error()；nil 接收者或空 Err 是 "httpx: transport error <nil>"。
// 例：(&TransportError{Err: io.EOF}).Error() → "EOF"；
// (*TransportError)(nil).Error() → "httpx: transport error <nil>"。
func (e *TransportError) Error() string {
	if e == nil || e.Err == nil {
		return "httpx: transport error <nil>"
	}
	return e.Err.Error()
}

// Unwrap 支持 errors.Is / errors.As 解到真正的网络错误。
// 输入：接收者可为 nil。
// 返回：底层 Err；nil 接收者或空 Err 返回 nil（不 panic）。
// 例：errors.Is(&TransportError{Err: io.EOF}, io.EOF) → true。
func (e *TransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
