// Command quickstart 是 gohttpkit 的最小可运行示例：
// 建一个客户端 → 打一个接口 → 读响应体与状态码，全程带 trace_id 的结构化日志。
//
//	go run ./examples/quickstart
//	go run ./examples/quickstart -url https://api.github.com/zen
//	HTTPKIT_LOG_LEVEL=debug go run ./examples/quickstart          # 看全量 http_transaction 日志
//	go run ./examples/quickstart -proxy socks5://127.0.0.1:1080   # 走代理
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

func main() {
	target := flag.String("url", "https://httpbin.org/get", "要请求的完整 URL")
	proxyURL := flag.String("proxy", "", "代理地址，如 socks5://127.0.0.1:1080")
	flag.Parse()

	u, err := url.Parse(*target)
	if err != nil {
		exit("解析 -url 失败: %v", err)
	}
	base := u.Scheme + "://" + u.Host
	path := u.RequestURI()

	// ① 构头器：本库唯一必填的接缝。这里用最简的 StaticHeaders；
	//    需要 cookie / token / 逐请求变化的头，就自己实现 HeaderProvider（见 examples/fidelity）。
	client, err := httpx.New(httpx.Options{
		Headers: httpx.StaticHeaders{
			Base: base,
			Headers: map[string]string{
				"accept":          "application/json,text/plain,*/*",
				"accept-language": "en-US,en;q=0.9",
				"user-agent":      "gohttpkit-quickstart/1.0",
			},
		},
		ProxyURL: *proxyURL,
	})
	if err != nil {
		exit("创建客户端失败: %v", err)
	}

	// ② 发请求。ctx 里没有 trace_id 时库会自动生成一个，本次请求的所有日志都带上它。
	ctx := logger.WithTraceID(context.Background(), "quickstart-001")
	body, err := client.Get(ctx, path, nil)
	if err != nil {
		exit("请求失败: %v", err)
	}

	// ③ 状态码走快照读（并发安全）。注意默认链【不会】因为非 2xx 而报错。
	logger.Info(ctx, "请求完成",
		slog.Int("status", client.SnapshotResponseStatusCode()),
		slog.Int("body_len", len(body)))

	fmt.Println("──────── 响应体 ────────")
	fmt.Println(preview(string(body), 800))
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n...(共 %d 字节)", len(s))
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
