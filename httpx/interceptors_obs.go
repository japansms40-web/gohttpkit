package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/japansms40-web/gohttpkit/internal/envx"
	"github.com/japansms40-web/gohttpkit/logger"
)

// interceptors_obs.go —— 观察类拦截器：日志、状态缓存、整包快照、HTML 落盘。
// 它们不改变请求/响应语义，只做记录。

// ─────────────────────────────── logging ───────────────────────────────

type loggingInterceptor struct{}

// NewLoggingInterceptor 打一条 http_transaction 结构化日志。
//
// 默认所有状态码都打全量字段（排查友好）；Options.LogSummaryOnly 置 true 后 2xx/3xx
// 只打 6 个摘要字段，4xx/5xx 仍然全量 —— 出问题的请求永远看得到细节。
//
// duration_ms 是端到端耗时（本层在 retry 之外，含全部重试与解压），即客户端视角的真实延迟。
// req_headers 打的是第一次尝试的头（见 bridge 里的快照说明）。
func NewLoggingInterceptor() Interceptor { return &loggingInterceptor{} }

func (i *loggingInterceptor) Intercept(ch *Chain) (*Response, error) {
	start := time.Now()
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}

	durMs := time.Since(start).Milliseconds()
	// 慢请求阈值（默认 0=关闭）：超阈值降级为 Warn 并标 slow=true，便于聚合告警。
	slowMS := envx.Int(envSlowMS, 0)
	slow := slowMS > 0 && durMs >= int64(slowMS)

	req := ch.Request()
	c := ch.Client()
	limit := c.LogBodyLimit()

	if resp.StatusCode >= 400 || !c.Options().LogSummaryOnly {
		attrs := []slog.Attr{
			slog.String("method", req.Method),
			slog.String("url", req.FullURL),
			slog.String("proxy", c.Options().ProxyURL),
			slog.Any("req_headers", req.ReqHeaders),
			slog.Any("req_params", req.Params),
			slog.String("req_body", string(TruncateBodyForLog(req.Body, limit))),
			slog.Int("req_body_len", len(req.Body)),
			slog.Int("status", resp.StatusCode),
			slog.String("proto", resp.Proto),
			slog.Int("proto_major", resp.ProtoMajor),
			slog.Any("resp_headers", resp.Header),
			slog.String("resp_body", string(TruncateBodyForLog(resp.Body, limit))),
			slog.Int("resp_body_len", len(resp.Body)),
			slog.Int64("duration_ms", durMs),
		}
		emit(req.Ctx, slow, attrs)
	} else {
		attrs := []slog.Attr{
			slog.String("method", req.Method),
			slog.String("url", req.FullURL),
			slog.Int("status", resp.StatusCode),
			slog.Int("req_body_len", len(req.Body)),
			slog.Int("resp_body_len", len(resp.Body)),
			slog.Int64("duration_ms", durMs),
		}
		emit(req.Ctx, slow, attrs)
	}
	return resp, nil
}

func emit(ctx context.Context, slow bool, attrs []slog.Attr) {
	if slow {
		logger.Warn(ctx, "http_transaction", append(attrs, slog.Bool("slow", true))...)
		return
	}
	logger.Info(ctx, "http_transaction", attrs...)
}

// ──────────────────────── 最近一次响应的状态码 / 响应头缓存 ────────────────────────

type statusCodeCacheInterceptor struct{}

// NewStatusCodeCacheInterceptor 记录最近一次响应的状态码，供 Client.SnapshotResponseStatusCode 读。
// 位于 retry 之外：重试跑完才记，中途的失败尝试不会污染缓存。
func NewStatusCodeCacheInterceptor() Interceptor { return &statusCodeCacheInterceptor{} }

func (i *statusCodeCacheInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	ch.Client().setLastStatusCode(resp.StatusCode)
	return resp, nil
}

type responseHeaderCacheInterceptor struct{}

// NewResponseHeaderCacheInterceptor 缓存最近一次响应头（供 Client.SnapshotResponseHeaders 读），
// 并调用 Options.OnResponseHeaders 做会话状态回写。它在默认链里，通常不需要你手动加。
//
// 无论状态码如何都会缓存：服务端在 400/403 时照样可能下发有用的新凭证，
// 只在 2xx 时缓存会把它们丢掉。
func NewResponseHeaderCacheInterceptor() Interceptor { return &responseHeaderCacheInterceptor{} }

func (i *responseHeaderCacheInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}

	// 先在本地 map 完整构建，再整体替换字段引用 —— 杜绝 make+Add 过程中被并发读到半成品。
	newHeaders := make(http.Header, len(resp.Header))
	for key, values := range resp.Header {
		for _, v := range values {
			newHeaders.Add(key, v)
		}
	}
	ch.Client().setLastHeaders(newHeaders)

	if hook := ch.Client().Options().OnResponseHeaders; hook != nil {
		hook(ch.Request().Ctx, newHeaders)
	}
	return resp, nil
}

// ─────────────────────────── 整包快照 / HTML 落盘（旁路） ───────────────────────────

// Transaction 一次请求/响应的完整快照（含请求头与体、响应头与体），供调试落盘。
//
// 【未脱敏】：cookie / authorization 原样保留，因为它的用途正是与真实抓包逐字段对比。
// 别把它写到会被共享或长期留存的地方。
type Transaction struct {
	Method      string              `json:"method"`
	URL         string              `json:"url"`
	Proxy       string              `json:"proxy"`
	ReqHeaders  map[string][]string `json:"req_headers"`
	ReqBody     string              `json:"req_body"`
	ReqBodyLen  int                 `json:"req_body_len"`
	Status      int                 `json:"status"`
	RespHeaders map[string][]string `json:"resp_headers"`
	RespBody    string              `json:"resp_body"`
	RespBodyLen int                 `json:"resp_body_len"`
	DurationMS  int64               `json:"duration_ms"`
}

type transactionInterceptor struct {
	SideChannelMarker
	sink func(*Transaction)
}

// NewTransactionInterceptor 把整链处理后的完整请求/响应快照交给 sink（落盘 / 存档）。
//
// 放在链【最外层】（首位）：此时 Request.ReqHeaders 已由 bridge 填好、Response.Body 是
// 最终对外的体。它被标记为 SideChannel，Client.WithChain 派生子链时会自动带过去。
// 出错（resp 为 nil）不回调。
func NewTransactionInterceptor(sink func(*Transaction)) Interceptor {
	return &transactionInterceptor{sink: sink}
}

func (i *transactionInterceptor) Intercept(ch *Chain) (*Response, error) {
	start := time.Now()
	resp, err := ch.Proceed()
	if i.sink != nil && resp != nil {
		req := ch.Request()
		i.sink(&Transaction{
			Method:      req.Method,
			URL:         req.FullURL,
			Proxy:       ch.Client().Options().ProxyURL,
			ReqHeaders:  map[string][]string(req.ReqHeaders),
			ReqBody:     string(req.Body),
			ReqBodyLen:  len(req.Body),
			Status:      resp.StatusCode,
			RespHeaders: map[string][]string(resp.Header),
			RespBody:    string(resp.Body),
			RespBodyLen: len(resp.Body),
			DurationMS:  time.Since(start).Milliseconds(),
		})
	}
	return resp, err
}

type htmlSaveInterceptor struct {
	SideChannelMarker
	sink func([]byte)
}

// NewHTMLSaveInterceptor 只保存 HTML 响应：content-type 含 text/html 时把响应体交给 sink，
// 不修改响应、无其它副作用。同样建议放链最外层，同样是 SideChannel。
func NewHTMLSaveInterceptor(sink func(html []byte)) Interceptor {
	return &htmlSaveInterceptor{sink: sink}
}

func (i *htmlSaveInterceptor) Intercept(ch *Chain) (*Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return resp, err // 出错不保存
	}
	if i.sink != nil && resp != nil &&
		strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "text/html") {
		i.sink(resp.Body)
	}
	return resp, nil
}
