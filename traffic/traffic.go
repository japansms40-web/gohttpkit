// Package traffic 提供对底层代理连接的真实流量计数能力。
//
// 设计目标:
//   - 零依赖、零侵入: 只依赖标准库; 未注入 Hook 时 WrapConn 直接返回原 conn,
//     report 在一次原子读后立即返回, 对上游使用者无任何行为变化与性能开销。
//   - 真实口径: 在 net.Conn(TCP)层计数, Read/Write 统计的是密文字节,
//     TLS 记录层、HTTP 头、wss 帧、TLS 握手全部天然计入, 贴近代理商真实计费。
//
// 归属(SysUserID)、Redis 累加、MQ 上报等业务逻辑全部由调用方(InsExecutor)
// 通过 SetHook 注入的回调实现, 本包不感知任何业务概念。
package traffic

import (
	"net"
	"sync/atomic"
)

// Hook 流量回调: 每次底层连接发生读或写时被调用, 上报本次的增量字节。
// 同一次调用中 bytesRead 与 bytesWritten 仅其一非零。
// 回调会在网络读写热路径上被高频触发, 实现必须极轻量(建议仅做本地原子累加)。
type Hook func(bytesRead, bytesWritten int64)

// hook 为全局可注入的回调指针。nil 表示未启用统计(零开销)。
var hook atomic.Pointer[Hook]

// SetHook 注入全局流量回调。传 nil 关闭统计。
func SetHook(h Hook) {
	if h == nil {
		hook.Store(nil)
		return
	}
	hook.Store(&h)
}

// report 在 hook 已注入时上报增量字节。
func report(r, w int64) {
	if p := hook.Load(); p != nil && *p != nil {
		(*p)(r, w)
	}
}

// WrapConn 在已建立的代理连接外包一层流量计数。
// hook 未注入或 c 为 nil 时直接返回原值(零开销、零行为变化)。
func WrapConn(c net.Conn) net.Conn {
	if c == nil || hook.Load() == nil {
		return c
	}
	return &statConn{Conn: c}
}

// statConn 包裹 net.Conn, 在每次 Read/Write 后按 TCP 层真实字节逐次上报。
// 逐次上报(而非 Close 时汇总)以保证 wss 等常驻长连接也能实时计量。
type statConn struct {
	net.Conn
}

func (s *statConn) Read(b []byte) (int, error) {
	n, err := s.Conn.Read(b)
	if n > 0 {
		report(int64(n), 0)
	}
	return n, err
}

func (s *statConn) Write(b []byte) (int, error) {
	n, err := s.Conn.Write(b)
	if n > 0 {
		report(0, int64(n))
	}
	return n, err
}
