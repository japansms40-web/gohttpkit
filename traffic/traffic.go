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
package traffic

import (
	"net"
	"sync/atomic"
)

// Hook 流量回调：每次底层连接发生读或写时被调用，上报本次的增量字节。
// 给进程启动时注入累加器的调用方（InsExecutor 等）；运行在 Read/Write 热路径，
// 不得阻塞、加锁或做 IO，建议只做本地原子累加。
// 输入：bytesRead / bytesWritten 为本次增量；经 Read/Write 触发时仅一侧非零。
// 返回：无。
type Hook func(bytesRead, bytesWritten int64)

// hook 为进程级全局回调指针。nil 表示未启用统计（零开销）。
// 并发模型：任意 goroutine 可 SetHook；WrapConn 与已包装连接的 Read/Write
// 通过 atomic.Load 观察当前 Hook。替换立即可见。回调必须无锁、无阻塞、无 IO。
var hook atomic.Pointer[Hook]

// SetHook 注入全局流量回调。传 nil 关闭统计。
// 给进程启动或测试复位；与 WrapConn 无先后强制，但先 Wrap 再 SetHook 不会追溯包装旧连接。
// 输入：h 为 nil 关闭；非 nil 替换当前回调。任意 goroutine 可调用。
// 返回：无。
// 例：SetHook(func(r, w int64){ read.Add(r) }) 后新连接开始计数；SetHook(nil) 关闭。
func SetHook(h Hook) {
	if h == nil {
		hook.Store(nil)
		return
	}
	hook.Store(&h)
}

// report 在 hook 已注入时上报增量字节。
// 输入：r / w 为本次增量，可为 0。
// 返回：无。hook 未设或内层函数值为 nil 则 no-op，不 panic。
func report(r, w int64) {
	if p := hook.Load(); p != nil && *p != nil {
		(*p)(r, w)
	}
}

// WrapConn 在已建立的代理连接外包一层流量计数。
// 给 netproxy 拨号成功路径；应用层一般不必直接调。已是 *statConn 则原样返回，避免双计。
// 输入：c 为 nil 原样返回；hook 未注入时返回原 conn（事后 SetHook 不追溯）。
// 返回：未启用或 nil → 原值；已包装 → 同一 *statConn；否则新 *statConn。
// 例：hook 已注入时 WrapConn(c) 得到 *statConn；再 WrapConn 一次仍是同一个。
func WrapConn(c net.Conn) net.Conn {
	if c == nil || hook.Load() == nil {
		return c
	}
	if sc, ok := c.(*statConn); ok {
		return sc
	}
	return &statConn{Conn: c}
}

// statConn 包裹 net.Conn, 在每次 Read/Write 后按 TCP 层真实字节逐次上报。
// 逐次上报(而非 Close 时汇总)以保证 wss 等常驻长连接也能实时计量。
type statConn struct {
	net.Conn
}

// Read 实现 net.Conn。
// 输入：b 为调用方缓冲，原样交给底层；nil / 空切片行为与底层一致。
// 返回：n / err 透传底层；n>0 才 report(n, 0)，即使同时带错。
func (s *statConn) Read(b []byte) (int, error) {
	n, err := s.Conn.Read(b)
	if n > 0 {
		report(int64(n), 0)
	}
	return n, err
}

// Write 实现 net.Conn。
// 输入：b 为调用方写出的字节，原样交给底层。
// 返回：n / err 透传底层；n>0 才 report(0, n)，即使同时带错。
func (s *statConn) Write(b []byte) (int, error) {
	n, err := s.Conn.Write(b)
	if n > 0 {
		report(0, int64(n))
	}
	return n, err
}
