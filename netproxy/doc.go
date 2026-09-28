// Package netproxy 提供代理拨号基建：把 socks5 / http / https 代理接到 http.Transport 上，
// 或直接拿到一个 socks5 的 proxy.Dialer 供 TCP 类链路（MQTT / WebSocket / 裸 TCP）使用。
//
// 所有经本包建立的连接都会过一层 traffic.WrapConn，未注入 traffic.Hook 时零开销、零行为变化；
// 注入后即可在 TCP 层拿到真实收发字节（含 TLS 握手与记录层开销），贴近代理商的计费口径。
//
// 配置 / 解析失败是本包类型错误（*UnsupportedProxySchemeError、*InvalidProxyURLError；
// dialer 未实现 proxy.ContextDialer 时 DialContextWithProxy 返回 *UnsupportedDialerError），
// 判定请用 errors.As，不要扫文案。拨号失败原样返回底层错误，重试识别在 httpx.Do。
//
// 文件结构：
//
//	netproxy/
//	├── doc.go     包文档（本文件）
//	├── errors.go  本包类型错误：UnsupportedProxySchemeError / InvalidProxyURLError / UnsupportedDialerError
//	├── proxy.go   ApplyProxyToTransport、ParseProxyURL、DialContextWithProxy
//	└── scheme.go  代理 Scheme 枚举与解析
package netproxy
