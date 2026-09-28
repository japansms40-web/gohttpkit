// Command quickstart 是 gohttpkit 的最小可运行示例：
// 建一个客户端 → 打一个接口 → 读响应体与状态码，全程带 trace_id 的结构化日志。
//
//	go run ./examples/quickstart
//	go run ./examples/quickstart -url https://api.github.com/zen
//	在 main 里 logger.SetConfig(logger.Config{Level: logger.LevelDebug}) 可看全量 event=http.transaction 日志
//	go run ./examples/quickstart -proxy socks5://127.0.0.1:1080   # 走代理
//
// 文件结构：
//
//	quickstart/
//	├── doc.go     包文档（本文件）
//	├── errors.go  本示例的 Op / Kind / Attr 定义
//	└── main.go    建客户端、打接口、打印响应
package main
