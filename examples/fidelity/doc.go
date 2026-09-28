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
//
// 文件结构：
//
//	fidelity/
//	├── doc.go     包文档（本文件）
//	├── errors.go  本示例的 Op 定义
//	└── main.go    浏览器头构建、cookie 回写、整包快照落盘 + 内置假服务器
package main
