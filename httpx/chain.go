package httpx

import (
	"context"
	"net/http"
	"net/url"
)

// chain.go —— OkHttp 风格拦截器链的公开框架。
//
// 与「把链藏在包内」的做法相反，本库刻意把 Interceptor / Chain / Request / Response
// 全部导出且字段可写：一个 HTTP 基建库真正的价值在于让接入方把自己的横切逻辑
// （请求签名、会话状态回写、业务错误归类、埋点、录制回放）插进链里，而不是只能用
// 库作者预设好的那几层。代价是链的结构成为对外契约，改动即破坏性变更 —— 这是有意的取舍。
//
// 响应在链上从最内层往外流：越早执行的后处理放得越内层。
//
//	外 ───────────────────────────────────────────────────────► 内
//	tracing → logging → bodyDecode → statusCodeCache → retry → bridge → callServer
//	                                                     ▲                  │
//	                                                     └─── 重试在此循环 ──┘
//	响应回流方向相反：callServer → bridge → retry → statusCodeCache → bodyDecode → logging → tracing

// Interceptor 链上的单个拦截器。实现 Intercept 时：
//   - 调用 ch.Proceed() 继续链，在其前后插入自己的逻辑；
//   - 不调用 Proceed 即为「终端拦截器」（唯一接触网络的那层，见 NewCallServerInterceptor）；
//   - 可以多次调用 Proceed（重试拦截器正是这么做的）。
type Interceptor interface {
	Intercept(*Chain) (*Response, error)
}

// InterceptorFunc 把普通函数适配成 Interceptor，省去为一次性逻辑定义类型。
type InterceptorFunc func(*Chain) (*Response, error)

// Intercept 把函数适配成 Interceptor。
// 给一次性逻辑：不想为单层定义类型时用 InterceptorFunc(fn)。
// 输入 ch：当前链游标，原样交给底层函数；f 为 nil 会 panic。
// 返回：底层函数的 (*Response, error)，本适配器不改写、不补默认。
// 例：InterceptorFunc(func(ch *Chain) (*Response, error) { return ch.Proceed() })。
func (f InterceptorFunc) Intercept(ch *Chain) (*Response, error) { return f(ch) }

// Interceptors 一条有序拦截器链（外 → 内），可整体注入 Options.Interceptors。
// 链的编辑用 presets.go 里的 InsertBefore / InsertAfter / Replace / Without / SpliceBeforeTerminal。
type Interceptors []Interceptor

// SideChannel 标记「旁路观察」拦截器：只把整链处理后的只读快照交给 sink（落盘 / 上报 / 打印），
// 无业务副作用，设计上恒放链首位。
//
// Client.WithChain 派生子链时会把父链最外层连续的这类拦截器自动前置到子链，
// 使录制 / 调试用的 sink 在专用子链（如禁重定向链）里继续生效。
// 自定义旁路拦截器内嵌 SideChannelMarker 即被识别。
type SideChannel interface {
	Interceptor
	SideChannel()
}

// SideChannelMarker 内嵌它即把自定义拦截器标记为旁路观察层。
type SideChannelMarker struct{}

// SideChannel 实现 SideChannel 接口的空方法。
// 给自定义旁路拦截器：内嵌 SideChannelMarker 即被 WithChain 识别为观察层。
// 输入：值接收者，无字段、无副作用。
// 返回：无。例：type rec struct { httpx.SideChannelMarker }。
func (SideChannelMarker) SideChannel() {}

// Request 一次请求在链上的可变状态。入口 Client.Do 构建，链上各层按需改写。
// 单次调用内串行使用，无并发访问 —— 拦截器可以放心直接读写字段。
type Request struct {
	// Ctx 本次请求的 context（Client.Do 入口已保证注入 trace_id）。
	Ctx context.Context
	// Method HTTP 方法。
	Method string
	// Path 请求路径（也可以是完整绝对 URL，此时跨域直发，见 Client.Do）。
	Path string
	// Params 查询参数，非空时已编码进 FullURL。
	Params url.Values
	// BaseURL HeaderProvider.BaseURL() 的结果，供 origin / referer 构建。
	BaseURL string
	// FullURL 实际请求的完整 URL（BaseURL + Path + ?Params，或 Path 本身）。
	FullURL string

	// Body 已编码的请求体字节。重试时由 bridge 重新 bytes.NewReader 重建。
	Body []byte

	// ExtraHeaders 逐请求附加头（覆盖构建值；值为空串的条目会被跳过，不是删除）。
	ExtraHeaders map[string]string
	// HeaderWhitelist 头白名单。
	//   nil    → 发送 HeaderProvider 构建的全量头（常规用法）
	//   非 nil → 严格白名单：只发列出的头（空 map = 一个都不发，仅 ExtraHeaders 在线）
	// value 为空串表示「取构建值」，非空表示「用这个固定值覆盖」。
	HeaderWhitelist map[string]string

	// Attempt 当前尝试序号（0-based），由 retry 拦截器写入、bridge 读取。
	Attempt int
	// ReqHeaders 第一次尝试的请求头快照，由 bridge 在 Attempt==0 时填充，供日志复用。
	ReqHeaders http.Header
	// HTTPReq 本次尝试的 *http.Request，由 bridge 构建、终端拦截器消费（每次尝试覆盖）。
	HTTPReq *http.Request
}

// Response 链上的响应。终端拦截器只填元数据与 Raw（体未读）；bodyDecode 解压读体后
// 填充 Body 并把 Raw 置 nil。位于 bodyDecode 之内（更靠近网络）的拦截器看到的 Body 恒为空。
type Response struct {
	// StatusCode HTTP 状态码。默认链不因非 2xx 报错，业务判断请读这个字段
	// 或 Client.SnapshotResponseStatusCode()，不要假设 err != nil。
	StatusCode int
	// Proto 协议字面量（如 "HTTP/2.0"），日志与指纹对比用。
	Proto string
	// ProtoMajor 协议主版本（2 表示 HTTP/2）。部分拦截器按它区分 h1/h2 行为。
	ProtoMajor int
	// Header 响应头。bodyDecode 之内的拦截器看到的是终端刚拿到的头；
	// 外层看到的仍是同一份引用，改它会影响后续观察层。
	Header http.Header
	// Raw 原始响应，终端拦截器填充；bodyDecode 读完体后关闭并置 nil。
	Raw *http.Response
	// Body 解压后的响应体，bodyDecode 之后有效；外层拦截器可原地替换（如 HTML 提纯）。
	Body []byte
}

// Chain 拦截器链的执行游标：Proceed 派生 index+1 的新游标调用下一个拦截器。
// 拦截器拿到的就是它，经 Request() / Client() 访问本次请求与客户端。
type Chain struct {
	client       *Client
	interceptors Interceptors
	index        int
	req          *Request
}

// Proceed 继续执行链上的下一个拦截器（最终落到终端发出网络请求）。
// 给拦截器实现：在自己的前后处理之间调用它；不调用即为终端。
// 输入：当前游标；不改写 ch 本身，派生 index+1 的新游标交给下一层。
// 返回：下一层的 (*Response, error)；游标越界是 *ChainExhaustedError{Index, Length}。
// 例：默认链走到终端 → 网络响应；空链 Proceed → *ChainExhaustedError{Index:0, Length:0}。
func (ch *Chain) Proceed() (*Response, error) {
	if ch.index >= len(ch.interceptors) {
		return nil, &ChainExhaustedError{Index: ch.index, Length: len(ch.interceptors)}
	}
	next := &Chain{
		client:       ch.client,
		interceptors: ch.interceptors,
		index:        ch.index + 1,
		req:          ch.req,
	}
	return ch.interceptors[ch.index].Intercept(next)
}

// Request 返回本次请求的可变状态。
// 给拦截器：读 method/url/body，或改 ExtraHeaders / HTTPReq。
// 输入：无。返回同一份 *Request（不复制）；单次调用内串行使用，可直接改字段。
// 例：ch.Request().Method → "GET"。
func (ch *Chain) Request() *Request { return ch.req }

// Client 返回发起本次请求的客户端。
// 给拦截器：读 HeaderProvider、RetryPolicy、Options，或写 Snapshot 缓存。
// 输入：无。返回同一份 *Client（不复制）；并发安全规则见 Client 头注。
// 例：ch.Client().RetryPolicy().MaxRetries。
func (ch *Chain) Client() *Client { return ch.client }
