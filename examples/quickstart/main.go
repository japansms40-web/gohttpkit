// Command quickstart 是 gohttpkit 的最小可运行示例：
// 建一个客户端 → 打一个接口 → 读响应体与状态码，全程带 trace_id 的结构化日志。
//
//	go run ./examples/quickstart
//	go run ./examples/quickstart -url https://api.github.com/zen
//	在 main 里 logger.SetConfig(logger.Config{Level: logger.LevelDebug}) 可看全量 event=http.transaction 日志
//	go run ./examples/quickstart -proxy socks5://127.0.0.1:1080   # 走代理
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/japansms40-web/gohttpkit/logger"
)

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 是 main 的可测试形态：输出写进 out，参数从 args 解析，失败返回 error 而不是 os.Exit。
// 把 main 写成这样几乎不增加复杂度，却让示例本身也能被测试覆盖。
func run(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("quickstart", flag.ContinueOnError)
	fs.SetOutput(out)
	target := fs.String("url", "https://httpbin.org/get", "要请求的完整 URL")
	proxyURL := fs.String("proxy", "", "代理地址，如 socks5://127.0.0.1:1080")
	if err := fs.Parse(args); err != nil {
		return err
	}

	u, err := url.Parse(*target)
	if err != nil {
		return fmt.Errorf("解析 -url 失败: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("-url 必须是完整 URL（含 scheme 与域名），got %q", *target)
	}
	base := u.Scheme + "://" + u.Host
	path := u.RequestURI()

	// ① 构头器：本库唯一必填的接缝。这里用最简的 StaticHeaders；
	//    需要 cookie / token / 逐请求变化的头，就自己实现 HeaderProvider（见 examples/fidelity）。
	client, err := interceptor.NewClient(httpx.Options{
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
		return fmt.Errorf("创建客户端失败: %w", err)
	}

	// ② 发请求。ctx 里没有 trace_id 时库会自动生成一个，本次请求的所有日志都带上它。
	ctx := logger.WithTraceID(context.Background(), "quickstart-001")
	body, err := client.Get(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}

	// ③ 状态码走快照读（并发安全）。注意默认链【不会】因为非 2xx 而报错。
	logger.Info(ctx, "请求完成",
		slog.Int("status", client.SnapshotResponseStatusCode()),
		slog.Int("body_len", len(body)))

	fmt.Fprintln(out, "──────── 响应体 ────────")
	fmt.Fprintln(out, preview(string(body), 800))
	return nil
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n...(共 %d 字节)", len(s))
}
