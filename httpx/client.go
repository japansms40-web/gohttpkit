// Package httpx 提供一套可直接复用的 HTTP 客户端基建：OkHttp 风格的拦截器链、
// 网络层重试与指数退避、四种压缩解码、代理接入、结构化日志、header 白名单精确发头、
// 请求/响应整包快照落盘。
//
// 它不绑定任何具体 API：请求头怎么构建由你实现 HeaderProvider 决定，业务错误怎么归类
// 由你写一层拦截器决定。库只负责把这些横切关注点组织成一条可预测、可测试、可扩展的链。
//
// 最小可用示例：
//
//	client, err := interceptor.NewClient(httpx.Options{
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

// client.go —— Client 构造与请求入口（New / Do / Get / Post* / Snapshot / WithChain）。
// 独立成文件：接入方第一眼看到的 API 与链框架、拦截器实现分开，避免打开包就陷进细节。
//
// 超时 / 重试 / 慢请求阈值在 New 时从 Options 固化；热路径不再回读。

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
	slowMS       int // 慢请求阈值（毫秒），New 时固化；0 = 关闭。

	lastMu         sync.RWMutex
	lastHeaders    http.Header
	lastStatusCode int
}

// New 创建客户端。
// 给接入方：每个会话一个 Client，绑定自己的 HeaderProvider。
// 输入 opts：Headers 必填；Timeout / ResponseHeaderTimeout / Retry 零值走代码默认；
// Interceptors 为 nil 或空切片时不装默认链（空链 Do 得到 *ChainExhaustedError）；
// 开箱即用请走 interceptor.NewClient。传了 Transport 则整份沿用（不再接代理、不再调优）。
// 返回：成功 *Client；Headers==nil 是 *MissingHeaderProviderError{Field:"Options.Headers"}；
// 自建 Transport 失败是 fmt.Errorf("httpx: build transport: %w", err)，里层仍是 netproxy 类型。
// 例：New(Options{Headers: StaticHeaders{Base: "https://api.example.com"}}) → (*Client, nil)；
// New(Options{}) → *MissingHeaderProviderError。
func New(opts Options) (*Client, error) {
	if opts.Headers == nil {
		return nil, &MissingHeaderProviderError{Field: "Options.Headers"}
	}

	transport := opts.Transport
	if transport == nil {
		t, err := newTransport(opts.ProxyURL, opts.responseHeaderTimeout())
		if err != nil {
			return nil, fmt.Errorf("httpx: build transport: %w", err)
		}
		transport = t
	}

	return &Client{
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   opts.timeout(),
		},
		headers:      opts.Headers,
		interceptors: opts.Interceptors,
		opts:         opts,
		retry:        normalizeRetry(opts.Retry),
		slowMS:       opts.slowMS(),
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
// 给业务代码：所有 HTTP 入口最终都落到这里。
// 输入 ctx：取消 / deadline / 已有 trace 原样沿用；没有非空 trace_id 时在此补一个，
// 不会改写调用方手里的原 ctx。要让 Do 前后的业务日志同链，须先 EnsureTraceID。
// 输入 spec：Method 空则 GET；Path 可以是相对路径或绝对 URL；Body 见 EncodeRequestBody。
// 返回：成功是链处理后的 body（非 2xx 也是 (body, nil)）；编码失败是 *RequestBodyEncodeError；
// 链上错误原样返回。空链是 *ChainExhaustedError，不再回落默认链。
// 例：client.Do(ctx, RequestSpec{Path: "/v1/ping"}) → (body, nil)；
// Body 不可 JSON 编码 → *RequestBodyEncodeError。
// 为什么不因非 2xx 报错：多数私有 API 用 200 之外的状态码承载有意义的业务响应体，
// 基建层替你判死会丢信息。想让非 2xx 变 error，挂 interceptor.NewStatusSemanticsInterceptor。
func (c *Client) Do(ctx context.Context, spec RequestSpec) ([]byte, error) {
	// 只补本次请求内的 trace，不 StartSpan、也不写回调用方的原 ctx。
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

	ch := &Chain{
		client:       c,
		interceptors: c.interceptors,
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
// 给只需路径 + 查询参数的调用方，等价于 Do(GET)。
// 输入 ctx / path / params：语义同 Do；params 为 nil 不带查询串。
// 返回：同 Do。例：client.Get(ctx, "/v1/ping", nil) → (body, nil)。
func (c *Client) Get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	return c.Do(ctx, RequestSpec{Method: http.MethodGet, Path: path, Params: params})
}

// PostForm 发起 POST 表单请求（全量头）。
// 给表单接口：data 走 EncodeRequestBody（url.Values 自动编码，string 保持自定义顺序）。
// 输入 ctx / path / data：data 为 nil 则空体 POST。
// 返回：同 Do。例：client.PostForm(ctx, "/login", url.Values{"u":{"a"}}) → (body, nil)。
func (c *Client) PostForm(ctx context.Context, path string, data any) ([]byte, error) {
	return c.Do(ctx, RequestSpec{Method: http.MethodPost, Path: path, Body: data})
}

// PostJSON 发起 POST JSON 请求（全量头 + content-type: application/json）。
// 给 JSON API：body 经 EncodeRequestBody 编码，并强制带上 application/json。
// 输入 ctx / path / body：body 不可编码时整次调用失败、不发出请求。
// 返回：同 Do。例：client.PostJSON(ctx, "/v1/item", map[string]any{"id":1}) → (body, nil)；
// body 为 chan 等无法 JSON 的类型 → *RequestBodyEncodeError。
func (c *Client) PostJSON(ctx context.Context, path string, body any) ([]byte, error) {
	return c.Do(ctx, RequestSpec{
		Method:       http.MethodPost,
		Path:         path,
		Body:         body,
		ExtraHeaders: map[string]string{headerContentType: mimeJSON},
	})
}

// Headers 返回本客户端的 HeaderProvider。
// 给调用方读回自己的会话状态（cookie、token），不是拷贝。
// 输入：无。返回构造时那份引用；并发安全由 HeaderProvider 实现保证。
// 例：client.Headers().BaseURL() → "https://api.example.com"。
func (c *Client) Headers() HeaderProvider { return c.headers }

// Interceptors 返回本客户端当前的拦截器链。
// 给诊断与链编辑：外层切片是副本，改切片本身不影响客户端；
// 切片里的 Interceptor 仍是同一份引用，改拦截器内部状态会共享。
// 输入：无。返回 append 出来的新切片。
// 例：len(client.Interceptors()) 等于构造时写入的链长。
func (c *Client) Interceptors() Interceptors {
	return append(Interceptors(nil), c.interceptors...)
}

// RetryPolicy 返回归一化后的重试策略。
// 给 retry 拦截器与诊断：New 时已补齐 MaxRetries / BaseBackoff / IsRetryable。
// 输入：无。返回值类型副本；改返回值不影响客户端。
// 例：NoRetry() 构造后 RetryPolicy().MaxRetries == 0。
func (c *Client) RetryPolicy() RetryPolicy { return c.retry }

// Options 返回构造时的选项。
// 给拦截器读 ProxyURL / ExitIP / ASN / OnResponseHeaders。
// 输入：无。返回外层 struct 副本；内部引用字段（Headers、Transport、回调、切片）仍指向同一对象。
// 例：client.Options().Timeout 是构造时写入的原值，不是归一化后的 timeout()。
func (c *Client) Options() Options { return c.opts }

// SetTimeout 设置整请求超时。
// 给需要临时拉长/缩短单次超时的调用方。
// 输入 timeout：直接写进共享 http.Client.Timeout；0 表示标准库「不超时」。
// 返回：无。不得与并发中的请求同时调用，否则其它 in-flight 请求的超时会被一起改掉。
// 例：client.SetTimeout(5*time.Second) 后后续 Do 使用 5s。
func (c *Client) SetTimeout(timeout time.Duration) { c.HTTPClient.Timeout = timeout }

// SlowMS 返回 New 时固化的慢请求阈值（毫秒）。
// 给 logging 拦截器：热路径读这里，不再回读 Options.SlowMS。
// 输入：接收者可为 nil。
// 返回：>0 是阈值；0 = 关闭；nil 接收者回落 0。
// 例：Options{SlowMS: 1500 * time.Millisecond} 构造后 SlowMS() == 1500。
func (c *Client) SlowMS() int {
	if c == nil {
		return 0
	}
	return c.slowMS
}

// LogBodyLimit 返回日志里协议 body 的截断上限。
// 给拦截器与长连接日志：统一从这里取 limit，把「截断开关」收敛成一处语义。
// 输入：接收者可为 nil。
// 返回：默认 LogBodyMaxBytes；DisableLogBodyTruncation 为 true 时 0（不截断）；nil 接收者回落默认。
// 例：默认客户端 → 4096；关掉截断 → 0。
func (c *Client) LogBodyLimit() int {
	if c == nil {
		return LogBodyMaxBytes // nil-safe：回落到「截断开启」
	}
	if c.opts.DisableLogBodyTruncation {
		return 0
	}
	return LogBodyMaxBytes
}

// SnapshotResponseHeaders 并发安全地返回最近一次响应的头。
// 给业务判断与会话回写核对：返回同一份只读引用，不复制；调用方不得修改。
// 输入：无。返回缓存里那份 http.Header；尚未发起过请求为 nil。
// 例：第一次 Do 之后 Get("content-type") 可读；改返回值会污染缓存，禁止。
// 写入端永远是「在新 map 上构建完再整体替换字段引用」，只读迭代不会 race。
func (c *Client) SnapshotResponseHeaders() http.Header {
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()
	return c.lastHeaders
}

// SnapshotResponseStatusCode 并发安全地返回最近一次响应的状态码。
// 给业务判断（如 statusCode == 404）；必须走本方法，不要直读字段。
// 输入：无。返回最近一次成功走到缓存层的状态码；尚未请求为 0。
// 例：404 响应后本方法返回 404，Do 的 error 仍为 nil。
func (c *Client) SnapshotResponseStatusCode() int {
	c.lastMu.RLock()
	defer c.lastMu.RUnlock()
	return c.lastStatusCode
}

// CacheStatusCode 写入最近一次响应状态码。
// 给 statusCodeCache 与自定义缓存层：在锁内整体替换 int。
// 输入 code：HTTP 状态码，0 也是合法写入（表示清掉或尚未有码）。
// 返回：无。不复制、不加校验。
func (c *Client) CacheStatusCode(code int) {
	c.lastMu.Lock()
	c.lastStatusCode = code
	c.lastMu.Unlock()
}

// CacheResponseHeaders 写入最近一次响应头。
// 给 responseHeaderCache 与自定义缓存层：调用方必须先在锁外建好完整 map，再交给这里整体替换引用。
// 输入 h：新的 header 引用，可为 nil（表示清空）；本函数不复制、不改 h。
// 返回：无。
func (c *Client) CacheResponseHeaders(h http.Header) {
	c.lastMu.Lock()
	c.lastHeaders = h
	c.lastMu.Unlock()
}

// WithChain 派生一个子客户端。
// 给需要换链但不换会话的流程：禁重定向读 302、拿原始 HTML、临时加录制。
// 输入 chain：子链主体；父链最外层连续 SideChannel 会自动前置到它前面。
// 返回：新 *Client，共享 HTTPClient 与 HeaderProvider，自带独立响应缓存（新锁，不拷贝父锁）。
// 例：client.WithChain(interceptor.NoRedirectChain())；chain 为 nil 时子客户端只有继承来的观察层。
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
		slowMS:       c.slowMS,
	}
}
