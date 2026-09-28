package interceptor

// errors.go —— 本包错误定义（Op / Kind / Attrs key），规范见 docs/CODE_STANDARDS.md §5.1。

// opSendRequest 重试拦截器发送失败（不可重试、ctx 结束或未包成 RetryableError 时）外层 *errors.Error 的 Op。
const opSendRequest = "interceptor.send_request"
