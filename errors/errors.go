// Package errors 提供与业务无关的网络错误基建：可重试错误包装、瞬时网络错误判定、
// HTTP 状态码错误。
//
// 设计取舍：判定用「错误文案关键词表」而非 net.Error.Temporary()——经过 SOCKS5 代理、
// TLS、HTTP/2 多层包装后，底层错误类型早已丢失，只有文案还留着线索。关键词表可经
// RegisterRetryableKeywords 扩展，也可经 httpx.RetryPolicy.IsRetryable 整体替换。
//
// 本包单文件即可：可重试错误与 HTTP 状态码错误体量都小，再拆只增加跳转。
// 测试专用快照见 export_test.go（仅 go test 编译，不进生产 API）。
package errors

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// RetryableError 可重试的网络错误。httpx 重试耗尽、或接入方按「稍后换出口再试」
// 处理瞬时失败时返回它；判定请用 errors.As，不要扫 Error() 文案。
type RetryableError struct {
	// Err 触发重试判定的那次底层错误（通常是网络发送失败）。
	Err error
	// Attempts 已经尝试的次数（含第一次；耗尽时 = MaxRetries+1）。
	Attempts int
	// LastError 最后一次尝试的错误，便于日志区分「最初原因」和「最后一次现象」。
	LastError error
}

// Error 用英文短句，便于跨语言接入方 grep 与 errors.Is 对照；次数与两次错误都保留，
// 因为「重试了几次、最后一次是什么」比「最初为什么失败」更常用来排障。
func (e *RetryableError) Error() string {
	return fmt.Sprintf("network error after %d attempts: %v (last error: %v)",
		e.Attempts, e.Err, e.LastError)
}

// Unwrap 返回底层错误，支持 errors.Is 和 errors.As
func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryableNetworkError 检测是否为可重试的网络错误
// 融合了两个实现：检查 RetryableError 类型和检查网络错误的关键词
func IsRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 首先检查是否为 RetryableError 类型
	var retryableErr *RetryableError
	if errors.As(err, &retryableErr) {
		return true
	}

	return matchRetryableKeyword(err.Error())
}

// matchRetryableKeyword 在读锁内扫关键词表。单独成函数是为了能用 defer 解锁，
// 命中或未命中都能解开，不必写两处 RUnlock。
func matchRetryableKeyword(msg string) bool {
	errStr := strings.ToLower(msg)
	keywordsMu.RLock()
	defer keywordsMu.RUnlock()
	for _, keyword := range retryableKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// keywordsMu 保护 retryableKeywords 的并发扩展（RegisterRetryableKeywords 可能在
// 任意 init/运行期被调用，而 IsRetryableNetworkError 在请求热路径上并发读）。
var keywordsMu sync.RWMutex

// retryableKeywords 可重试的网络错误关键词（小写匹配 err.Error()）。
// 这张表来自真实代理出网环境的长期积累，勿轻易删条目；扩展走 RegisterRetryableKeywords。
var retryableKeywords = []string{
	"connection reset by peer",
	"eof",
	"timeout",
	"context deadline exceeded", // ctx 超时（不含 "timeout" 子串，须单列，否则永不命中）
	"broken pipe",
	"connection refused",
	"connection aborted",
	"network is unreachable",
	"no such host",     // DNS 解析失败
	"host unreachable", // 代理/目标主机不可达（network is unreachable 之外的变体）
	"temporary failure",
	"i/o timeout",
	"network error after",             // 来自 RetryableError 的错误消息
	"tls: handshake failure",          // TLS 握手失败
	"tls: failed to verify",           // TLS 证书验证失败
	"bad record mac",                  // TLS 记录层 MAC 校验失败（代理链路串包/连接复用，账号无辜）
	"received record with version",    // tls: received record with version（代理串包，账号无辜）
	"first record does not look like", // SOCKS 出口被中间盒/captive portal 替换响应，非 TLS 字节序
	"x509: certificate is not",        // X.509 证书错误
	"unknown error",                   // 未知的网络错误
	"socks connect",                   // SOCKS 代理连接错误
	"with body length 0",              // ContentLength 与 Body 长度不匹配的错误
	"net.operror",                     // 包装层把 *net.OpError 类型名写进文案；小写匹配 "net.OpError"
}

// RegisterRetryableKeywords 追加可重试错误关键词（小写）。用于接入方遇到本表未覆盖的
// 代理/网络栈错误文案时就地扩展，无需 fork 本库。重复条目不去重（匹配是子串扫描，无害）。
func RegisterRetryableKeywords(keywords ...string) {
	if len(keywords) == 0 {
		return
	}
	lowered := make([]string, 0, len(keywords))
	for _, k := range keywords {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			lowered = append(lowered, k)
		}
	}
	keywordsMu.Lock()
	defer keywordsMu.Unlock()
	retryableKeywords = append(retryableKeywords, lowered...)
}

// retryableKeywordsSnapshot 返回当前关键词表的副本。生产代码不导出；
// 测试经 export_test.go 的 RetryableKeywords 调用。
func retryableKeywordsSnapshot() []string {
	keywordsMu.RLock()
	defer keywordsMu.RUnlock()
	return append([]string(nil), retryableKeywords...)
}

// HTTPStatusError 非 2xx 响应错误。httpx 的 statusSemantics 拦截器在「非 2xx 且响应体为空」
// 时返回它；调用方可用 errors.As 取出状态码与响应体做进一步判断。
//
// 注意：默认链【不含】statusSemantics——本库默认把非 2xx 原样交给调用方（body, nil），
// 与状态码一起经 Client.SnapshotResponseStatusCode() 读取。
type HTTPStatusError struct {
	// StatusCode HTTP 状态码（调用方应用它做分支，而不是解析 Error() 里的数字）。
	StatusCode int
	// Body 响应体快照。空体是 statusSemantics 默认规则的触发条件；HTML 错误页路径
	// 可能只放命中的 marker，便于日志短、errors.As 后仍能读到状态码。
	Body []byte
}

// FormatHTTPStatus 把状态码与响正文案收成库内统一格式。
// httpx 的 RetryableTextRule / HTML 错误页与本类型的 Error() 必须走这里，
// 否则接入方一半 errors.As、一半扫字符串，两种判定会漂。
func FormatHTTPStatus(status int, body []byte) string {
	return fmt.Sprintf("http error: %d, body: %s", status, body)
}

// Error 实现 error；格式由 FormatHTTPStatus 单一出处保证。
func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "http error: <nil>"
	}
	return FormatHTTPStatus(e.StatusCode, e.Body)
}

// IsHTTPStatus 判断 err 链上是否存在指定状态码的 HTTPStatusError。
func IsHTTPStatus(err error, status int) bool {
	var se *HTTPStatusError
	return errors.As(err, &se) && se.StatusCode == status
}
