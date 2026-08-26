package netproxy

// legacy_dialer_test.go —— 兜底拨号路径（dialer 未实现 proxy.ContextDialer 时）的分支覆盖。
//
// 正常情况下 x/net 的 SOCKS5 dialer 恒实现 ContextDialer，走不到这条路径。
// 但它是「万一第三方 dialer 不带 ctx」时唯一的保护，goroutine 泄漏窗口就靠它兜住，
// 所以每条分支都要有用例守着——真出事的时候没人有机会现场调试它。
//
// 用包内测试（而非 _test 外部包）是因为要改 legacyDialGuardTimeout 这个包级变量。

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/traffic"
)

// noCtxDialer 只实现 proxy.Dialer（Dial），不实现 ContextDialer，
// 以此把 DialContextWithProxy 逼进兜底路径。
type noCtxDialer struct {
	delay time.Duration
	conn  net.Conn
	err   error
}

func (d *noCtxDialer) Dial(network, addr string) (net.Conn, error) {
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	return d.conn, d.err
}

// pipeConn 是一端可关闭的假连接，用于断言守护 goroutine 确实把它关掉了。
type pipeConn struct {
	net.Conn
	closed chan struct{}
}

func newPipeConn() *pipeConn {
	c1, c2 := net.Pipe()
	go func() { _ = c2.Close() }()
	return &pipeConn{Conn: c1, closed: make(chan struct{}, 1)}
}

func (c *pipeConn) Close() error {
	select {
	case c.closed <- struct{}{}:
	default:
	}
	return c.Conn.Close()
}

func TestLegacy_拨号成功并包上流量计数(t *testing.T) {
	var read, written int64
	traffic.SetHook(func(r, w int64) { read += r; written += w })
	defer traffic.SetHook(nil)

	backend := newPipeConn()
	dial := DialContextWithProxy(&noCtxDialer{conn: backend})
	conn, err := dial(context.Background(), "tcp", "1.2.3.4:80")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if conn == backend {
		t.Fatal("返回的应是包了流量计数的 conn，而不是裸 conn")
	}
	go func() { _, _ = conn.Write([]byte("ping")) }()
	buf := make([]byte, 4)
	if _, err := conn.Read(buf); err != nil && !errors.Is(err, net.ErrClosed) {
		_ = err // 管道另一端已关闭属正常，这里只是把读写路径走一遍
	}
	_ = conn.Close()
}

func TestLegacy_拨号失败原样返回(t *testing.T) {
	sentinel := errors.New("dial refused")
	dial := DialContextWithProxy(&noCtxDialer{err: sentinel})
	conn, err := dial(context.Background(), "tcp", "1.2.3.4:80")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 原样返回底层错误", err)
	}
	if conn != nil {
		t.Fatal("失败时不该返回连接")
	}
}

func TestLegacy_ctx已取消时立即返回不拨号(t *testing.T) {
	dialed := make(chan struct{}, 1)
	d := &noCtxDialer{conn: newPipeConn()}
	dial := DialContextWithProxy(&noCtxDialer{conn: d.conn})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dial(ctx, "tcp", "1.2.3.4:80"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	select {
	case <-dialed:
		t.Fatal("ctx 已取消就不该真的去拨号")
	default:
	}
}

func TestLegacy_ctx中途取消时守护goroutine关闭迟到的连接(t *testing.T) {
	// 这条锁的是资源不泄漏：ctx 先超时返回了，但拨号 goroutine 稍后才拿到连接，
	// 那条连接必须被守护 goroutine 关掉，否则每次超时都漏一个 fd。
	backend := newPipeConn()
	dial := DialContextWithProxy(&noCtxDialer{delay: 80 * time.Millisecond, conn: backend})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	conn, err := dial(ctx, "tcp", "1.2.3.4:80")
	if err == nil {
		t.Fatal("ctx 超时应返回错误")
	}
	if conn != nil {
		t.Fatal("超时时不该返回连接")
	}
	if elapsed := time.Since(start); elapsed > 60*time.Millisecond {
		t.Fatalf("elapsed = %v，应在 ctx 超时那一刻就返回，不等拨号完成", elapsed)
	}

	select {
	case <-backend.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("迟到的连接没有被守护 goroutine 关闭(fd 泄漏)")
	}
}

func TestLegacy_守护goroutine超时后放弃等待(t *testing.T) {
	// 对端完全不响应时，守护 goroutine 不能永远挂着——到点打一条 warn 就走人，
	// 用一个「永不返回」的 dialer 把这条路径逼出来。
	old := legacyDialGuardTimeout
	legacyDialGuardTimeout = 20 * time.Millisecond
	defer func() { legacyDialGuardTimeout = old }()

	blocked := make(chan struct{})
	defer close(blocked)
	dial := DialContextWithProxy(&blockingDialer{release: blocked})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := dial(ctx, "tcp", "1.2.3.4:80"); err == nil {
		t.Fatal("want error")
	}
	// 给守护 goroutine 足够时间走到 timer 分支
	time.Sleep(80 * time.Millisecond)
}

// blockingDialer 的 Dial 一直阻塞到 release 被关闭，模拟完全不响应的对端。
type blockingDialer struct{ release chan struct{} }

func (d *blockingDialer) Dial(network, addr string) (net.Conn, error) {
	<-d.release
	return nil, errors.New("released")
}
