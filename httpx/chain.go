package httpx

import (
	"context"
	"errors"
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
//	logging → bodyDecode → statusCodeCache → retry → bridge → callServer
//	                                          ▲                  │
//	                                          └─── 重试在此循环 ──┘
//	响应回流方向相反：callServer → bridge → retry → statusCodeCache → bodyDecode → logging

// errChainExhausted 链尾没有终端拦截器（组装错误）。
var errChainExhausted = errors.New("httpx: 拦截器链已耗尽——链尾缺少终端拦截器(如 NewCallServerInterceptor)")

// Interceptor 链上的单个拦截器。实现 Intercept 时：
//   - 调用 ch.Proceed() 继续链，在其前后插入自己的逻辑；
//   - 不调用 Proceed 即为「终端拦截器」（唯一接触网络的那层，见 NewCallServerInterceptor）；
//   - 可以多次调用 Proceed（重试拦截器正是这么做的）。
type Interceptor interface {
	Intercept(*Chain) (*Response, error)
}

// InterceptorFunc 把普通函数适配成 Interceptor，省去为一次性逻辑定义类型。
type InterceptorFunc func(*Chain) (*Response, error)

// Intercept 实现 Interceptor。
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

// SideChannel 实现 SideChannel 接口。
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
	StatusCode int
	Proto      string
	ProtoMajor int
	Header     http.Header
	// Raw 原始响应，终端拦截器填充；bodyDecode 读完体后关闭并置 nil。
	Raw *http.Response
	// Body 解压后的响应体，bodyDecode 之后有效；外层拦截器可原地替换（如 HTML 提纯）。
	Body []byte
}

// TransportError 标记「网络发送本身失败」的错误，只有它才会被 retry 拦截器纳入可重试判断；
// 其它错误（构建请求失败、构头失败等）原样穿透、立即返回。
//
// 自定义终端拦截器必须把 http.Client.Do 的错误包成 *TransportError，否则重试层看不见它。
//
// 约束：位于 retry 与终端之间的拦截器不得再包装（wrap）该错误 —— retry 用 errors.As 解包，
// 多包一层虽仍能识别，但那段区间语义上只应透传。
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// Chain 拦截器链的执行游标：Proceed 派生 index+1 的新游标调用下一个拦截器。
// 拦截器拿到的就是它，经 Request() / Client() 访问本次请求与客户端。
type Chain struct {
	client       *Client
	interceptors Interceptors
	index        int
	req          *Request
}

// Proceed 继续执行链上的下一个拦截器（最终落到终端拦截器发出网络请求）。
func (ch *Chain) Proceed() (*Response, error) {
	if ch.index >= len(ch.interceptors) {
		return nil, errChainExhausted
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
func (ch *Chain) Request() *Request { return ch.req }

// Client 返回发起本次请求的客户端。
func (ch *Chain) Client() *Client { return ch.client }
