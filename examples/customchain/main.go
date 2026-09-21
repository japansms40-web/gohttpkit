// Command customchain 演示怎么把自己的横切逻辑插进拦截器链。
//
// 这里挂了四层，覆盖了绝大多数真实需求的形态：
//   - 请求签名：对最终 URL + body 计算签名头（必须在 bridge 之后才拿得到最终请求）
//   - 出网计数：只在真实发送时 +1（含每次重试），用于限速 / 计费
//   - 状态回写：把服务端下发的新 token 写回构头器，下一次请求自动带上
//   - 业务归类：把 {"status":"fail"} 统一翻译成 sentinel error，业务代码只 errors.Is
//
// 示例跑在一个内置的假服务器上，不需要外网：
//
//	go run ./examples/customchain
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

// ErrAccountBanned 业务 sentinel：由归类拦截器产出，业务代码只需 errors.Is 判断。
var ErrAccountBanned = errors.New("account banned")

// sessionHeaders 带会话状态的构头器：token 会被响应回写改写。
type sessionHeaders struct {
	base  string
	mu    sync.RWMutex
	token string
}

func (h *sessionHeaders) BuildHeaders(context.Context) map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]string{
		"accept":        "application/json",
		"user-agent":    "gohttpkit-customchain/1.0",
		"authorization": "Bearer " + h.token,
	}
}
func (h *sessionHeaders) BaseURL() string { return h.base }
func (h *sessionHeaders) setToken(v string) {
	h.mu.Lock()
	h.token = v
	h.mu.Unlock()
}

func main() {
	srv := fakeServer()
	defer srv.Close()

	hp := &sessionHeaders{base: srv.URL, token: "initial-token"}
	var sends atomic.Int64

	// ── 一、请求签名：插在终端之前，此时 bridge 已经把最终 URL 与头都准备好了
	signer := interceptor.NewRequestMutatorInterceptor(func(r *httpx.Request) {
		mac := hmac.New(sha256.New, []byte("secret-key"))
		mac.Write([]byte(r.Method + r.FullURL))
		mac.Write(r.Body)
		r.HTTPReq.Header["x-signature"] = []string{hex.EncodeToString(mac.Sum(nil))}
	})

	// ── 二、出网计数：同样在终端之前，所以每次重试也会被计到
	counter := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		sends.Add(1)
		return ch.Proceed()
	})

	// ── 三、状态回写：挂在 Options 上（会话级），换链也不会失效
	writeback := func(_ context.Context, h http.Header) {
		if v := h.Get("x-refresh-token"); v != "" {
			fmt.Printf("  ↻ 服务端下发新 token，已回写: %s\n", v)
			hp.setToken(v)
		}
	}

	// ── 四、业务错误归类：2xx 与非 2xx 用不同强度的规则
	classify := func(_ int, body []byte) error {
		var payload struct {
			Status string `json:"status"`
			Code   string `json:"code"`
		}
		// 2xx 只认结构化字段。用关键词去扫 2xx 响应体，迟早会被用户生成内容误伤。
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil
		}
		if payload.Status == "fail" && payload.Code == "banned" {
			return fmt.Errorf("%w (code=%s)", ErrAccountBanned, payload.Code)
		}
		return nil
	}

	chain := httpx.Prepend(interceptor.DefaultChain(),
		interceptor.NewClassifyInterceptor(classify), // 最外层：只在内层判定成功时才轮到它
	)
	chain = httpx.SpliceBeforeTerminal(chain, counter, signer) // 最内层：包住每次真实发送

	client, err := httpx.New(httpx.Options{
		Headers:           hp,
		Interceptors:      chain,
		OnResponseHeaders: writeback,
	})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	fmt.Println("① 正常请求（观察签名头与 token 回写）")
	body, err := client.Get(ctx, "/api/profile", nil)
	fmt.Printf("  err=%v body=%s\n", err, strings.TrimSpace(string(body)))

	fmt.Println("② 第二次请求（应带上回写后的新 token）")
	body, _ = client.Get(ctx, "/api/profile", nil)
	fmt.Printf("  body=%s\n", strings.TrimSpace(string(body)))

	fmt.Println("③ 命中业务错误（HTTP 200，但业务判定为封禁）")
	_, err = client.Get(ctx, "/api/banned", nil)
	fmt.Printf("  errors.Is(err, ErrAccountBanned) = %v  (%v)\n", errors.Is(err, ErrAccountBanned), err)

	fmt.Printf("\n真实出网次数: %d\n", sends.Load())
}

func fakeServer() *httptest.Server {
	var seq atomic.Int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/banned") {
			// 注意是 200：业务失败与 HTTP 失败是两回事，这正是归类拦截器存在的理由
			_, _ = w.Write([]byte(`{"status":"fail","code":"banned"}`))
			return
		}
		w.Header().Set("x-refresh-token", fmt.Sprintf("token-%d", seq.Add(1)))
		_, _ = fmt.Fprintf(w, `{"status":"ok","auth":%q,"signed":%v}`,
			r.Header.Get("Authorization"), r.Header.Get("X-Signature") != "")
	}))
}
