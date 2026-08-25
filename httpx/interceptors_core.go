package httpx

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/logger"
)

// interceptors_core.go —— 构成默认链骨架的核心拦截器。
// 顺序即语义：bodyDecode 在 retry 之外（解码失败不重试），statusCodeCache 在 retry 之外
// （重试中途不缓存状态码），bridge 在 retry 之内（每次重试重建请求与请求头）。

// ─────────────────────────────── retry ───────────────────────────────

type retryInterceptor struct{}

// NewRetryInterceptor 网络层重试：包住 bridge + 终端，每次重试重新构建请求与请求头。
// 策略取自 Client.RetryPolicy()（Options.Retry 或 env）。
//
// 只对 *TransportError 做可重试判断；bridge 产生的错误（构建请求失败、构头失败）
// 原样穿透立即返回 —— 那是代码/配置错误，重试一万次也是同样结果。
func NewRetryInterceptor() Interceptor { return &retryInterceptor{} }

func (i *retryInterceptor) Intercept(ch *Chain) (*Response, error) {
	policy := ch.Client().RetryPolicy()
	req := ch.Request()

	var lastErr error
	retryStart := time.Now()

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		req.Attempt = attempt

		resp, err := ch.Proceed()
		if err == nil {
			// 重试之后才成功时补一条确认日志，便于观察重试到底有没有用。
			if attempt > 0 {
				logger.Info(req.Ctx, "request retry succeeded",
					slog.String("method", req.Method),
					slog.String("url", req.FullURL),
					slog.Int("attempt", attempt+1),
					slog.Int64("duration_ms", time.Since(retryStart).Milliseconds()))
			}
			return resp, nil
		}

		var terr *TransportError
		if !stderrors.As(err, &terr) {
			return nil, err // 非传输层错误，原样穿透
		}
		doErr := terr.Err
		lastErr = doErr

		if !policy.IsRetryable(doErr) {
			return nil, fmt.Errorf("failed to send request: %w", doErr)
		}

		if attempt == policy.MaxRetries {
			return nil, &errors.RetryableError{Err: doErr, Attempts: attempt + 1, LastError: lastErr}
		}

		backoff := policy.BaseBackoff * (1 << uint(attempt))
		if policy.MaxBackoff > 0 && backoff > policy.MaxBackoff {
			backoff = policy.MaxBackoff
		}

		logger.Warn(req.Ctx, "request failed, retrying",
			slog.String("method", req.Method),
			slog.String("url", req.FullURL),
			slog.Any("req_headers", req.ReqHeaders),
			slog.Any("req_params", req.Params),
			slog.String("req_body", string(TruncateBodyForLog(req.Body, ch.Client().LogBodyLimit()))),
			slog.Int("req_body_len", len(req.Body)),
			slog.Int("attempt", attempt+1),
			slog.Int("max_retries", policy.MaxRetries),
			slog.Duration("backoff", backoff),
			slog.String("err_type", fmt.Sprintf("%T", doErr)),
			logger.Err(doErr))

		select {
		case <-req.Ctx.Done():
			return nil, &errors.RetryableError{Err: req.Ctx.Err(), Attempts: attempt + 1, LastError: lastErr}
		case <-time.After(backoff):
		}
	}

	// 循环内要么成功返回要么错误返回，走到这里说明逻辑被改坏了，兜底成错误而不是 nil。
	return nil, fmt.Errorf("failed to send request: %w", lastErr)
}

// ─────────────────────────────── bridge ───────────────────────────────

type bridgeInterceptor struct{}

// NewBridgeInterceptor 每次尝试重建 *http.Request 并构建/过滤请求头（OkHttp BridgeInterceptor 对位）。
// 必须位于 retry 之内，否则重试时用的是过期的头（比如已经轮转过的 token）。
func NewBridgeInterceptor() Interceptor { return &bridgeInterceptor{} }

func (i *bridgeInterceptor) Intercept(ch *Chain) (*Response, error) {
	req := ch.Request()
	c := ch.Client()

	// 每次重试重建 body reader（上一次尝试已经把 reader 读干了）
	var reqBody io.Reader
	if len(req.Body) > 0 {
		reqBody = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(req.Ctx, req.Method, req.FullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	built := c.Headers().BuildHeaders(req.Ctx)
	if built == nil {
		return nil, fmt.Errorf("httpx: HeaderProvider.BuildHeaders 返回 nil(构头失败)")
	}

	// 拷一份再改：BuildHeaders 的实现可能返回自己持有的内部 map，就地改写会污染后续请求
	// （尤其是并发场景下的 UA 缓存表）。这份拷贝同时把 key 统一成小写。
	allHeaders := make(map[string]string, len(built)+2)
	for k, v := range built {
		allHeaders[strings.ToLower(k)] = v
	}

	if !c.Options().DisableOriginReferer {
		origin, referer := BuildOriginAndReferer(req.BaseURL, req.Path)
		allHeaders["origin"] = origin
		allHeaders["referer"] = referer
	}

	// nil 白名单 = 全量发送；非 nil（含空 map）= 严格白名单。
	// 这一对语义是本库与「白名单必填」式实现的关键区别，见 Request.HeaderWhitelist 文档。
	strict := req.HeaderWhitelist != nil
	final := allHeaders
	if strict {
		final = FilterHeadersByWhitelist(allHeaders, req.HeaderWhitelist)
	}

	// extraHeaders 覆盖构建值；空值【跳过】而不是删除该头（要删头写一层拦截器）。
	for key, value := range req.ExtraHeaders {
		if value == "" {
			continue
		}
		final[strings.ToLower(key)] = value
	}

	// 标准库对少数几个头有特殊处理，且是按【规范化大写】key 识别的。我们坚持全小写写头
	// （真实浏览器/移动端在 HTTP/2 上就是小写，大写会成为指纹差异），于是必须自己处理这几个，
	// 否则 HTTP/1.1 下会发出重复头、或泄漏 Go 的默认 User-Agent。
	// HTTP/2 侧标准库用大小写不敏感比较，本来就没问题；这里的处理对 h2 无副作用。
	applySpecialHeaders(httpReq, final, strict)

	// 绕过 http.Header.Set 的 CanonicalMIMEHeaderKey，保留全小写 key。
	for key, value := range final {
		httpReq.Header[key] = []string{value}
	}

	req.HTTPReq = httpReq
	return ch.Proceed()
}

// ─────────────────────────── 终端（唯一接触网络的层）───────────────────────────

type callServerInterceptor struct{}

// NewCallServerInterceptor 终端拦截器：发出请求，不调用 Proceed。
// Do 的错误包成 *TransportError 供 retry 识别。
//
// 响应体的所有权在这里【移交】给 bodyDecode 拦截器（它读完体后负责 Close）。
// 因此自组的链若不含 NewBodyDecodeInterceptor，就必须自己关闭 Response.Raw.Body，
// 否则连接不会归还连接池，最终耗尽 fd。
func NewCallServerInterceptor() Interceptor { return &callServerInterceptor{} }

func (i *callServerInterceptor) Intercept(ch *Chain) (*Response, error) {
	SnapshotRequestHeaders(ch.Request())
	//nolint:bodyclose // 所有权移交 bodyDecodeInterceptor，由它读完体后 Close（见上方 doc）
	resp, err := ch.Client().HTTPClient.Do(ch.Request().HTTPReq)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	return newResponseFrom(resp), nil
}

type noRedirectCallServerInterceptor struct{}

// NewNoRedirectCallServerInterceptor 禁重定向的终端拦截器：用一份浅拷贝 *http.Client
// （仅覆盖 CheckRedirect，Transport/Timeout 沿用）发请求，把 302 原样交给调用方
// （读 Location、读 302 上的 Set-Cookie）。
//
// 为什么独立成一层而不是给终端加个开关：「跟不跟重定向」是某条链的语义，
// 由链里挑哪个终端来表达，不污染共享的终端实现。浅拷贝是每次 Intercept 一份栈对象，
// 绝不写共享的 Client.HTTPClient —— 并发安全。
func NewNoRedirectCallServerInterceptor() Interceptor { return &noRedirectCallServerInterceptor{} }

func (i *noRedirectCallServerInterceptor) Intercept(ch *Chain) (*Response, error) {
	SnapshotRequestHeaders(ch.Request())
	nr := *ch.Client().HTTPClient
	nr.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	//nolint:bodyclose // 同 callServerInterceptor：所有权移交 bodyDecodeInterceptor
	resp, err := nr.Do(ch.Request().HTTPReq)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	return newResponseFrom(resp), nil
}

// SnapshotRequestHeaders 在第一次尝试时留一份「最终发出的请求头」快照到 Request.ReqHeaders，
// 供日志与整包快照使用。终端拦截器在发出请求前调用；自定义终端也应调用它，否则日志里
// 的 req_headers 会是空的。
//
// 只在 Attempt==0 时取：一次调用的日志里 req_headers 应当是同一份，不因重试而前后不一致。
//
// 快照做了两处修正，让它如实反映线上字节：
//   - 剔除那条空值的规范化 User-Agent —— 它只是用来抑制标准库默认 UA 的开关，
//     并不会出现在线上，留在快照里只会让你和抓包对比时白白排查半天；
//   - 补回 host —— 它被写进了 Request.Host 而不在 Header map 里，但线上确实有这一行。
func SnapshotRequestHeaders(req *Request) {
	if req == nil || req.Attempt != 0 || req.HTTPReq == nil {
		return
	}
	snapshot := make(http.Header, len(req.HTTPReq.Header)+1)
	for k, v := range req.HTTPReq.Header {
		if k == "User-Agent" && len(v) == 1 && v[0] == "" {
			continue
		}
		snapshot[k] = v
	}
	if req.HTTPReq.Host != "" {
		snapshot["host"] = []string{req.HTTPReq.Host}
	} else if req.HTTPReq.URL != nil {
		snapshot["host"] = []string{req.HTTPReq.URL.Host}
	}
	req.ReqHeaders = snapshot
}

func newResponseFrom(resp *http.Response) *Response {
	return &Response{
		StatusCode: resp.StatusCode,
		Proto:      resp.Proto,
		ProtoMajor: resp.ProtoMajor,
		Header:     resp.Header,
		Raw:        resp,
	}
}

// ─────────────────────────────── bodyDecode ───────────────────────────────

type bodyDecodeInterceptor struct{}

// NewBodyDecodeInterceptor 读取响应体并按 content-encoding 解码（zstd / gzip / deflate / br）。
//
// 解码能力必须覆盖请求头里 accept-encoding 声明的集合，否则服务端按协商返回压缩流时，
// 下游拿到的是二进制乱码，JSON 解析与错误归类全线失配 —— 这类 bug 极难排查，因为
// 只在特定服务端/特定接口上偶发。
//
// 位置：在 retry 之外（解码错误不触发重试），在 statusCodeCache 之外。
func NewBodyDecodeInterceptor() Interceptor { return &bodyDecodeInterceptor{} }

func (i *bodyDecodeInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if resp.Raw == nil {
		return resp, nil // 已被自定义终端/回放层填好体，无事可做
	}

	raw := resp.Raw
	defer func() {
		if cerr := raw.Body.Close(); cerr != nil {
			logger.Error(ch.Request().Ctx, "failed to close response body", logger.Err(cerr))
		}
	}()

	var reader io.Reader = raw.Body
	switch strings.ToLower(resp.Header.Get("content-encoding")) {
	case "zstd":
		zr, zerr := zstd.NewReader(raw.Body)
		if zerr != nil {
			return nil, fmt.Errorf("failed to create zstd reader: %w", zerr)
		}
		defer zr.Close()
		reader = zr
	case "gzip":
		gz, gerr := gzip.NewReader(raw.Body)
		if gerr != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", gerr)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	case "deflate":
		fr := flate.NewReader(raw.Body)
		defer func() { _ = fr.Close() }()
		reader = fr
	case "br":
		// brotli.Reader 无 Close（不持额外资源，raw.Body 的 Close 已在 defer）
		reader = brotli.NewReader(raw.Body)
	}

	body, rerr := io.ReadAll(reader)
	if rerr != nil {
		// 读体中途超时/断连属瞬时网络错误，但本层在 retry 之外、无法自动重试；
		// 包成 RetryableError 让调用方按「网络类失败」处理（换代理续跑），
		// 而不是误判成硬失败去标记账号/任务不可用。
		if errors.IsRetryableNetworkError(rerr) {
			return nil, &errors.RetryableError{Err: rerr, Attempts: 1, LastError: rerr}
		}
		return nil, fmt.Errorf("failed to read response: %w", rerr)
	}

	resp.Body = body
	resp.Raw = nil
	return resp, nil
}

// applySpecialHeaders 处理标准库特殊对待的头，使「全小写写头」在 HTTP/1.1 与 HTTP/2 下
// 都产生同一份、且不重复的线上字节。final 里被本函数消化掉的条目会被删除。
//
//	host             → 写进 Request.Host（h1 从这里出 Host 行，h2 从这里出 :authority）
//	content-length   → 写进 Request.ContentLength（标准库负责输出，重复写会被服务端判为畸形）
//	user-agent       → 用一个空值的规范化条目抑制标准库默认 UA，只留我们自己那条小写的
//
// strict 为 true（严格白名单）且白名单里没有 user-agent 时，连标准库默认 UA 一起抑制：
// 「我告诉你发哪些头」就该字面成立，不该被偷偷塞一个 Go-http-client/1.1 进去。
func applySpecialHeaders(httpReq *http.Request, final map[string]string, strict bool) {
	if v, ok := final["host"]; ok {
		httpReq.Host = v
		delete(final, "host")
	}
	if v, ok := final["content-length"]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			httpReq.ContentLength = n
		}
		delete(final, "content-length")
	}
	_, hasUA := final["user-agent"]
	if hasUA || strict {
		// 空值 + 规范化 key：标准库据此认为「调用方已指定 UA 且为空」，于是不输出默认 UA，
		// 同时这条空记录本身也不会被写到线上（它在请求写出的排除集里）。
		httpReq.Header["User-Agent"] = []string{""}
	}
}
