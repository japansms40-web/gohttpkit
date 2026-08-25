// Package httpx 提供一套可直接复用的 HTTP 客户端基建：OkHttp 风格的拦截器链、
// 网络层重试与指数退避、四种压缩解码、代理接入、结构化日志、header 白名单精确发头、
// 请求/响应整包快照落盘。
//
// 它不绑定任何具体 API：请求头怎么构建由你实现 HeaderProvider 决定，业务错误怎么归类
// 由你写一层拦截器决定。库只负责把这些横切关注点组织成一条可预测、可测试、可扩展的链。
//
// 最小可用示例：
//
//	client, err := httpx.New(httpx.Options{
//	    Headers: httpx.StaticHeaders{
//	        Base:    "https://api.example.com",
//	        Headers: map[string]string{"accept": "application/json"},
//	    },
//	})
//	body, err := client.Get(ctx, "/v1/ping", nil)
//
// 更多用法见 examples/ 下三个可直接 go run 的例子。
package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/japansms40-web/gohttpkit/logger"
)

// Client 一个 HTTP 客户端实例。
//
// 并发模型：单个 *Client 可被多 goroutine 并发调用。
//   - lastHeaders / lastStatusCode 受 lastMu 保护，外部读取必须走
//     SnapshotResponseHeaders() / SnapshotResponseStatusCode()，禁止直读字段。
//   - HeaderProvider 的并发安全由其实现方保证（见 HeaderProvider 文档）。
//
// 跨会话不可共享：Client 与它的 HeaderProvider 绑定，而 HeaderProvider 通常携带
// 某个账号/会话的身份（cookie、token），复用会串号。一个会话一个 Client。
type Client struct {
	// HTTPClient 底层标准库客户端。导出以便接入方做特殊处理（如临时改 CheckRedirect），
	// 但请注意它被本 Client 的所有请求共享，就地改写会影响并发中的其它请求。
	HTTPClient *http.Client

	headers      HeaderProvider
	interceptors Interceptors
	opts         Options
	retry        RetryPolicy

	lastMu         sync.RWMutex
	lastHeaders    http.Header
	lastStatusCode int
}

// New 创建客户端。Options.Headers 必填。
func New(opts Options) (*Client, error) {
	if opts.Headers == nil {
		return nil, fmt.Errorf("httpx: Options.Headers 必填(实现 HeaderProvider，最简可用 httpx.StaticHeaders)")
	}

	transport := opts.Transport
	if transport == nil {
		t, err := NewTransport(opts.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("httpx: 构建 transport 失败: %w", err)
		}
		transport = t
	}

	interceptors := opts.Interceptors
	if interceptors == nil {
		interceptors = DefaultChain()
	}

	return &Client{
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   opts.timeout(),
		},
		headers:      opts.Headers,
		interceptors: interceptors,
		opts:         opts,
		retry:        opts.Retry.normalized(),
	}, nil
}

// RequestSpec 描述一次请求。除 Method / Path 外都可省略。
type RequestSpec struct {
	// Method HTTP 方法，空则 GET。
	Method string
	// Path 请求路径（"/v1/x"），也可以是完整绝对 URL（"https://other.example.com/x"）
	// 以发起跨域请求 —— 此时 origin / referer 仍按 HeaderProvider.BaseURL() 构建。
	Path string
	// Params 查询参数。
	Params url.Values
	// Body 请求体，支持 nil / []byte / string / url.Values / map / struct，见 EncodeRequestBody。
	Body any
	// ExtraHeaders 逐请求附加头，覆盖构建值。值为空串的条目会被【跳过】而不是删除该头
	// （要删头请写一层拦截器 delete(ch.Request().HTTPReq.Header, "cookie")）。
	ExtraHeaders map[string]string
	// HeaderWhitelist 头白名单：
	//   nil    → 发送 HeaderProvider 构建的全量头（默认，常规用法）
	//   非 nil → 严格白名单，只发列出的头；空 map 就一个都不发（高保真复刻抓包用）
	HeaderWhitelist map[string]string
}

// Do 执行一次请求，返回经整条链处理后的响应体。
//
// 注意本方法【不因非 2xx 报错】：默认链把状态码原样交给调用方，用
// SnapshotResponseStatusCode() 读。想让非 2xx 直接变成 error，挂一层
// NewStatusSemanticsInterceptor。这个取舍是刻意的——多数私有 API 会用 200 之外的
// 状态码承载有意义的业务响应体，基建层替你判死会丢信息。
func (c *Client) Do(ctx context.Context, spec RequestSpec) ([]byte, error) {
	// trace_id 兜底：所有请求的单一汇点，调用方未注入时此处生成，保证请求内日志可关联。
	ctx = logger.EnsureTraceID(ctx)

	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}

	baseURL := c.headers.BaseURL()
	fullURL := baseURL + spec.Path
	if strings.HasPrefix(spec.Path, "http://") || strings.HasPrefix(spec.Path, "https://") {
		fullURL = spec.Path
	}
	if len(spec.Params) > 0 {
		fullURL += "?" + spec.Params.Encode()
	}

	bodyData, err := EncodeRequestBody(spec.Body)
	if err != nil {
		return nil, err
	}

	interceptors := c.interceptors
	if len(interceptors) == 0 {
		interceptors = DefaultChain()
	}

	ch := &Chain{
		client:       c,
		interceptors: interceptors,
		req: &Request{
			Ctx:             ctx,
			Method:          method,
			Path:            spec.Path,
			Params:          spec.Params,
			BaseURL:         baseURL,
			FullURL:         fullURL,
			Body:            bodyData,
			ExtraHeaders:    spec.ExtraHeaders,
			HeaderWhitelist: spec.HeaderWhitelist,
		},
	}

	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Get 发起 GET 请求（全量头）。
func (c *Client) Get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	return c.Do(ctx, RequestSpec{Method: http.MethodGet, Path: path, Params: params})
}

// PostForm 发起 POST 表单请求（全量头）。data 支持 url.Values（自动编码）或 string（保持自定义顺序）。
func (c *Client) PostForm(ctx context.Context, path string, data any) ([]byte, error) {
	return c.Do(ctx, RequestSpec{Method: http.MethodPost, Path: path, Body: data})
}

// PostJSON 发起 POST JSON 请求（全量头 + content-type: application/json）。
func (c *Client) PostJSON(ctx context.Context, path string, body any) ([]byte, error) {
	return c.Do(ctx, RequestSpec{
		Method:       http.MethodPost,
		Path:         path,
		Body:         body,
		ExtraHeaders: map[string]string{"content-type": "application/json"},
	})
}

// Headers 返回本客户端的 HeaderProvider（便于调用方读回自己的会话状态）。
func (c *Client) Headers() HeaderProvider { return c.headers }

// Interceptors 返回本客户端当前的拦截器链（副本，改它不影响客户端）。
func (c *Client) Interceptors() Interceptors {
	return append(Interceptors(nil), c.interceptors...)
}

// RetryPolicy 返回归一化后的重试策略（供 retry 拦截器与诊断使用）。
func (c *Client) RetryPolicy() RetryPolicy { return c.retry }

// Options 返回构造时的选项（副本语义：Options 内的引用字段仍指向同一对象）。
func (c *Client) Options() Options { return c.opts }

// SetTimeout 设置整请求超时。
func (c *Client) SetTimeout(timeout time.Duration) { c.HTTPClient.Timeout = timeout }

// LogBodyLimit 返回日志里协议 body 的截断上限：默认 LogBodyMaxBytes，
// Options.DisableLogBodyTruncation 为 true 时返回 0（不截断）。
// 拦截器与长连接日志统一走这里取 limit，把「截断开关」收敛成一处语义。
func (c *Client) LogBodyLimit() int {
	if c == nil {
		return LogBodyMaxBytes // nil-safe：回落到「截断开启」
	}
	if c.opts.DisableLogBodyTruncation {
		return 0
	}
	return LogBodyMaxBytes
}

// SnapshotResponseHeaders 并发安全地返回最近一次响应的头。返回 nil 表示尚未发起过请求。
//
// 写入端永远是「在新 map 上构建完再整体替换字段引用」，所以返回的 map 自身不会再被改写，
// 拿到后做 Get / Values / 迭代等只读操作不会触发 race。
func (c *Client) SnapshotResponseHeaders() http.Header {
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()
	return c.lastHeaders
}

// SnapshotResponseStatusCode 并发安全地返回最近一次响应的状态码。
// 业务判断（如 statusCode == 404）必须走本方法，不要直读字段。
func (c *Client) SnapshotResponseStatusCode() int {
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()
	return c.lastStatusCode
}

// setLastResponse 由 statusCodeCache / responseHeaderCache 拦截器调用。
func (c *Client) setLastStatusCode(code int) {
	c.lastMu.Lock()
	c.lastStatusCode = code
	c.lastMu.Unlock()
}

func (c *Client) setLastHeaders(h http.Header) {
	c.lastMu.Lock()
	c.lastHeaders = h
	c.lastMu.Unlock()
}

// WithChain 派生一个子客户端：共享同一 HTTPClient 与 HeaderProvider（故会话状态互通），
// 但用传入的链，且自带独立的「最近一次响应」缓存（新锁，不拷贝父锁）。
//
// 典型用途：主链跑业务，派生子链跑一段需要不同处理的流程（禁重定向读 302 Location、
// 拿原始 HTML 而不做提纯、临时加一层录制）。父客户端的默认链完全不受影响。
//
// 父链最外层【连续的】SideChannel 拦截器会被自动前置到子链，让录制/调试 sink 继续生效。
func (c *Client) WithChain(chain Interceptors) *Client {
	var observers Interceptors
	for _, it := range c.interceptors {
		if _, ok := it.(SideChannel); !ok {
			break // 只取最外层连续前缀，遇到首个业务拦截器即停
		}
		observers = append(observers, it)
	}
	derived := append(append(Interceptors{}, observers...), chain...)

	return &Client{
		HTTPClient:   c.HTTPClient,
		headers:      c.headers,
		interceptors: derived,
		opts:         c.opts,
		retry:        c.retry,
	}
}
