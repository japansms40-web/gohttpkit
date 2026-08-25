// Command fidelity 演示「高保真复刻」——让程序发出的请求和抓包里的真实请求逐字节一致。
//
// 这是本库最核心、也最容易被低估的能力。复刻一个真实客户端时，决定成败的往往不是业务逻辑，
// 而是这些细节：
//
//   - 只发抓包里出现过的那些头，一个不多（多一个 accept-encoding 就可能被判为非官方客户端）
//   - 头名全小写（真实 HTTP/2 客户端就是小写，Sec-Ch-Ua 这种大写形态是明显的机器特征）
//   - cookie 手工按抓包顺序拼（map 遍历顺序随机，用标准库 cookie jar 顺序就变了）
//   - 表单参数保持抓包顺序（url.Values.Encode() 会按字典序重排）
//   - 服务端下发的新 token / cookie 立刻回写，下一次请求带上
//   - 每次请求整包落盘，事后与抓包逐字段对比
//
// 示例跑在内置假服务器上，不需要外网：
//
//	go run ./examples/fidelity
//	go run ./examples/fidelity -dump ./out    # 把每次请求的完整快照落盘
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────── 一、按抓包复刻的构头器 ───────────────────────

// browserHeaders 复刻某个真实浏览器会话的请求头与 cookie。
//
// 并发模型：单个客户端会被多 goroutine 共享，csrfToken / cookies 会被响应回写改写，
// 故所有可变状态经 mu 保护；BuildHeaders 在读锁内拼好整串再返回。
type browserHeaders struct {
	base string

	mu        sync.RWMutex
	csrfToken string
	sessionID string
	extraCk   map[string]string
}

// cookieOrder 是抓包里 Cookie 头的字段顺序。
//
// 为什么要写死顺序：Go 的 map 遍历顺序每次都不同，直接拼出来的 cookie 串顺序随机，
// 而真实浏览器的 cookie 顺序是稳定的（按写入时间/路径特异性）。顺序不稳定本身就是特征。
var cookieOrder = []string{"datr", "mid", "csrftoken", "sessionid", "ds_user_id"}

func (h *browserHeaders) BuildHeaders(context.Context) map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 头名全小写，与真实 HTTP/2 抓包一致
	return map[string]string{
		"accept":             "*/*",
		"accept-language":    "en-US,en;q=0.9",
		"content-type":       "application/x-www-form-urlencoded",
		"cookie":             h.buildCookieLocked(),
		"sec-ch-ua":          `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"user-agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
		"x-csrftoken":        h.csrfToken,
		"x-requested-with":   "XMLHttpRequest",
	}
}

// buildCookieLocked 按固定顺序手工拼 cookie（调用方需持有读锁）。
func (h *browserHeaders) buildCookieLocked() string {
	values := map[string]string{
		"csrftoken": h.csrfToken,
		"sessionid": h.sessionID,
	}
	for k, v := range h.extraCk {
		values[k] = v
	}
	var sb strings.Builder
	for _, name := range cookieOrder {
		v, ok := values[name]
		if !ok || v == "" {
			continue // 抓包里没有值的字段就整条不发，而不是发一个 name=
		}
		if sb.Len() > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(name)
		sb.WriteByte('=')
		sb.WriteString(v)
	}
	return sb.String()
}

func (h *browserHeaders) BaseURL() string { return h.base }

// applySetCookie 把响应里的 Set-Cookie 回写进会话状态。
// 漏了这一步的典型症状：整条链路全程 200，但业务就是不成功 —— 因为你一直在用过期的 token。
func (h *browserHeaders) applySetCookie(resp http.Header) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, raw := range resp.Values("Set-Cookie") {
		name, value, ok := parseSetCookie(raw)
		if !ok || value == "" {
			continue
		}
		switch name {
		case "csrftoken":
			h.csrfToken = value
		case "sessionid":
			h.sessionID = value
		default:
			if h.extraCk == nil {
				h.extraCk = map[string]string{}
			}
			h.extraCk[name] = value
		}
	}
}

func parseSetCookie(raw string) (name, value string, ok bool) {
	pair, _, _ := strings.Cut(raw, ";")
	name, value, ok = strings.Cut(strings.TrimSpace(pair), "=")
	return strings.TrimSpace(name), strings.TrimSpace(value), ok
}

// ─────────────────────── 二、按抓包定义的端点白名单 ───────────────────────

// 每个端点只发抓包里出现过的那些头。value 为空 = 取构头值，非空 = 用这个固定值。
//
// 顺序说明：Go 的 http.Header 是 map，本库无法控制头在线上的【顺序】，只能保证
// 「发哪些头、值是什么、大小写如何」精确可控。绝大多数服务端不校验头顺序；
// 若你的目标真的校验，那需要自定义 Transport 直接写 HTTP/2 帧，超出本库范围。
var (
	endpointFetchPage = map[string]string{
		"accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"accept-language":    "",
		"sec-ch-ua":          "",
		"sec-ch-ua-mobile":   "",
		"sec-ch-ua-platform": "",
		"sec-fetch-dest":     "document",
		"sec-fetch-mode":     "navigate",
		"sec-fetch-site":     "none",
		"user-agent":         "",
	}
	endpointSubmit = map[string]string{
		"accept":           "",
		"content-type":     "",
		"cookie":           "",
		"origin":           "",
		"referer":          "",
		"sec-fetch-dest":   "",
		"sec-fetch-mode":   "",
		"sec-fetch-site":   "",
		"user-agent":       "",
		"x-csrftoken":      "",
		"x-requested-with": "",
	}
)

func main() {
	dumpDir := flag.String("dump", "", "把每次请求的完整快照落盘到该目录")
	flag.Parse()

	srv := fakeServer()
	defer srv.Close()

	hp := &browserHeaders{base: srv.URL, extraCk: map[string]string{"datr": "captured-datr", "mid": "captured-mid"}}

	var step atomic.Int32
	chain := httpx.Prepend(httpx.DefaultChain(),
		// 整包快照：请求/响应头与体全留档，事后与抓包逐字段对比。
		// 它被标记为旁路观察层，WithChain 派生子链时会自动跟着走。
		httpx.NewTransactionInterceptor(func(tx *httpx.Transaction) {
			n := step.Add(1)
			printTransaction(n, tx)
			if *dumpDir != "" {
				dump(*dumpDir, n, tx)
			}
		}),
	)

	client, err := httpx.New(httpx.Options{
		Headers:      hp,
		Interceptors: chain,
		// Set-Cookie 回写挂在 Options 上：下面 WithChain 派生禁重定向子链时，
		// 回写照样生效 —— 会话状态是客户端的责任，不是某条链的责任。
		OnResponseHeaders: func(_ context.Context, h http.Header) { hp.applySetCookie(h) },
	})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	// ① 抓主页：拿 csrftoken。注意用【禁重定向】的派生子链 —— 这一步真实抓包里是 302，
	//    跟随重定向会把 Location 与 302 上的 Set-Cookie 一起吃掉。
	fmt.Println("═══ step 1: GET / （禁重定向，读 302 与 Set-Cookie）═══")
	noRedirect := client.WithChain(httpx.NoRedirectChain())
	if _, err := noRedirect.Do(ctx, httpx.RequestSpec{
		Path:            "/",
		HeaderWhitelist: endpointFetchPage,
	}); err != nil {
		panic(err)
	}
	fmt.Printf("  status=%d location=%q\n\n",
		noRedirect.SnapshotResponseStatusCode(), noRedirect.SnapshotResponseHeaders().Get("location"))

	// ② 提交表单：body 手工拼，保持抓包里的参数顺序（url.Values 会按字典序重排）。
	fmt.Println("═══ step 2: POST /api/submit （白名单精确发头 + 手工保序 body）═══")
	body := "user_id=42&action=confirm&timestamp=1700000000&nonce=abc123"
	respBody, err := client.Do(ctx, httpx.RequestSpec{
		Method:          http.MethodPost,
		Path:            "/api/submit",
		Body:            body,
		HeaderWhitelist: endpointSubmit,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("  响应: %s\n", strings.TrimSpace(string(respBody)))
}

func printTransaction(n int32, tx *httpx.Transaction) {
	keys := make([]string, 0, len(tx.ReqHeaders))
	for k := range tx.ReqHeaders {
		keys = append(keys, k)
	}
	fmt.Printf("  [snapshot %02d] %s %s → %d\n", n, tx.Method, tx.URL, tx.Status)
	fmt.Printf("               发出 %d 个请求头: %s\n", len(keys), strings.Join(keys, ", "))
}

func dump(dir string, n int32, tx *httpx.Transaction) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "落盘失败: %v\n", err)
		return
	}
	data, _ := json.MarshalIndent(tx, "", "  ")
	path := filepath.Join(dir, fmt.Sprintf("step%03d_transaction.json", n))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "落盘失败: %v\n", err)
		return
	}
	fmt.Printf("               已落盘 %s\n", path)
}

func fakeServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Add("set-cookie", "csrftoken=fresh-csrf-9f2a; Path=/; Secure")
			w.Header().Add("set-cookie", "sessionid=sess-771; Path=/; HttpOnly")
			w.Header().Set("location", "/home")
			w.WriteHeader(http.StatusFound)
		case "/api/submit":
			w.Header().Set("content-type", "application/json")
			_, _ = fmt.Fprintf(w, `{"ok":true,"got_csrf":%q,"got_cookie":%q}`,
				r.Header.Get("X-Csrftoken"), r.Header.Get("Cookie"))
		default:
			_, _ = w.Write([]byte("home"))
		}
	}))
}
