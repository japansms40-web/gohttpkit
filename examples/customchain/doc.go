// Command customchain 演示怎么把自己的横切逻辑插进拦截器链。
//
// 这里挂了四层，覆盖了绝大多数真实需求的形态：
//   - 请求签名：对最终 URL + body 计算签名头（必须在 bridge 之后才拿得到最终请求）
//   - 出网计数：只在真实发送时 +1（含每次重试），用于限速 / 计费
//   - 状态回写：把服务端下发的新 token 写回构头器，下一次请求自动带上
//   - 业务归类：把 {"status":"fail"} 翻译成带 Kind 的结构化错误，业务代码用 IsKind
//
// 示例跑在一个内置的假服务器上，不需要外网：
//
//	go run ./examples/customchain
//
// 文件结构：
//
//	customchain/
//	├── doc.go     包文档（本文件）
//	├── errors.go  本示例的 Op / Kind 定义
//	└── main.go    四层自定义拦截器 + 内置假服务器
package main
