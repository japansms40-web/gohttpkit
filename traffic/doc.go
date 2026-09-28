// Package traffic 提供对底层代理连接的真实流量计数能力。
//
// 设计目标:
//   - 零依赖、零侵入: 只依赖标准库; 未注入 Hook 时 WrapConn 直接返回原 conn,
//     report 在一次原子读后立即返回, 对上游使用者无任何行为变化与性能开销。
//   - 真实口径: 在 net.Conn(TCP)层计数, Read/Write 统计的是密文字节,
//     TLS 记录层、HTTP 头、wss 帧、TLS 握手全部天然计入, 贴近代理商真实计费。
//
// 谁调用：进程启动时 SetHook（如 InsExecutor 注入累加/MQ）；netproxy 拨号成功后 WrapConn。
// HTTP/HTTPS 代理走标准库 CONNECT，不经本包。归属、Redis、MQ 等业务逻辑全部由 Hook 实现。
//
// 文件结构：
//
//	traffic/
//	├── doc.go      包文档（本文件）
//	└── traffic.go  Hook 注入（SetHook）与计数连接包装 WrapConn
package traffic
