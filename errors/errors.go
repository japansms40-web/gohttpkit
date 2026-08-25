// Package errors 提供与业务无关的网络错误基建：可重试错误包装、瞬时网络错误判定、
// HTTP 状态码错误。
//
// 设计取舍：判定用「错误文案关键词表」而非 net.Error.Temporary()——经过 SOCKS5 代理、
// TLS、HTTP/2 多层包装后，底层错误类型早已丢失，只有文案还留着线索。关键词表可经
// RegisterRetryableKeywords 扩展，也可经 httpx.RetryPolicy.IsRetryable 整体替换。
package errors

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// RetryableError 可重试的网络错误
type RetryableError struct {
	Err       error // 原始错误
	Attempts  int   // 已经尝试的次数
	LastError error // 最后一次尝试的错误
}

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

	errStr := strings.ToLower(err.Error())

	keywordsMu.RLock()
	for _, keyword := range retryableKeywords {
		if strings.Contains(errStr, keyword) {
			keywordsMu.RUnlock()
			return true
		}
	}
	keywordsMu.RUnlock()

	// 检查是否为临时网络错误（net.Error 接口）
	if strings.Contains(err.Error(), "net.OpError") {
		// 如果是网络操作错误，可能是临时错误，基于错误信息判断
		return true
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
	retryableKeywords = append(retryableKeywords, lowered...)
	keywordsMu.Unlock()
}

// RetryableKeywords 返回当前关键词表的快照（只读，供诊断/测试）。
func RetryableKeywords() []string {
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
	StatusCode int
	Body       []byte
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("http error: %d, body: %s", e.StatusCode, string(e.Body))
}

// IsHTTPStatus 判断 err 链上是否存在指定状态码的 HTTPStatusError。
func IsHTTPStatus(err error, status int) bool {
	var se *HTTPStatusError
	return errors.As(err, &se) && se.StatusCode == status
}
