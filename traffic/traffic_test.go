package traffic

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resetHook 在每个用例结束后把全局 hook 复位, 避免用例间互相污染。
func resetHook(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { hook.Store(nil) })
}

// fakeConn 是一个最小化的 net.Conn 实现, Read/Write 返回预设的字节数与错误,
// 其余方法均为满足接口的空实现。
type fakeConn struct {
	readN    int
	readErr  error
	writeN   int
	writeErr error
}

func (f *fakeConn) Read(b []byte) (int, error)         { return f.readN, f.readErr }
func (f *fakeConn) Write(b []byte) (int, error)        { return f.writeN, f.writeErr }
func (f *fakeConn) Close() error                       { return nil }
func (f *fakeConn) LocalAddr() net.Addr                { return nil }
func (f *fakeConn) RemoteAddr() net.Addr               { return nil }
func (f *fakeConn) SetDeadline(t time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func TestSetHook_传入nil关闭统计(t *testing.T) {
	resetHook(t)

	SetHook(func(r, w int64) {})
	if hook.Load() == nil {
		t.Fatal("注入非 nil hook 后 hook.Load() 不应为 nil")
	}

	SetHook(nil)
	if hook.Load() != nil {
		t.Fatal("传入 nil 应关闭统计, hook.Load() 应为 nil")
	}
}

func TestReport_未注入时noop(t *testing.T) {
	resetHook(t)
	// 未注入 hook 时调用 report 不应 panic。
	report(1, 2)
}

func TestReport_已注入时透传(t *testing.T) {
	resetHook(t)

	var gotR, gotW int64
	SetHook(func(r, w int64) {
		gotR, gotW = r, w
	})

	report(123, 456)
	t.Logf("report(123,456) → r=%d w=%d", gotR, gotW)
	if gotR != 123 || gotW != 456 {
		t.Fatalf("report 透传错误: got (%d, %d), want (123, 456)", gotR, gotW)
	}
}

func TestWrapConn_nil连接原样返回(t *testing.T) {
	resetHook(t)
	SetHook(func(r, w int64) {})

	if got := WrapConn(nil); got != nil {
		t.Fatalf("c 为 nil 时应原样返回 nil, got %v", got)
	}
}

func TestWrapConn_未注入返回原连接(t *testing.T) {
	resetHook(t)

	c := &fakeConn{}
	got := WrapConn(c)
	if got != net.Conn(c) {
		t.Fatal("hook 未注入时应原样返回原 conn")
	}
	if _, ok := got.(*statConn); ok {
		t.Fatal("hook 未注入时不应包裹成 statConn")
	}
}

func TestWrapConn_已注入则包装(t *testing.T) {
	resetHook(t)
	SetHook(func(r, w int64) {})

	c := &fakeConn{}
	got := WrapConn(c)
	sc, ok := got.(*statConn)
	if !ok {
		t.Fatalf("hook 注入后应包裹成 *statConn, got %T", got)
	}
	if sc.Conn != net.Conn(c) {
		t.Fatal("statConn 应包裹原始 conn")
	}
}

func TestStatConn_读路径(t *testing.T) {
	wantErr := errors.New("read boom")
	tests := []struct {
		name      string
		readN     int
		readErr   error
		wantR     int64
		reported  bool
		wantErrIs error
	}{
		{name: "正常读上报", readN: 10, wantR: 10, reported: true},
		{name: "读到0不上报", readN: 0, reported: false},
		{name: "有数据且有错也上报", readN: 5, readErr: wantErr, wantR: 5, reported: true, wantErrIs: wantErr},
		{name: "0字节带错不上报", readN: 0, readErr: wantErr, reported: false, wantErrIs: wantErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetHook(t)
			var gotR, gotW int64
			var called bool
			SetHook(func(r, w int64) {
				called = true
				gotR, gotW = r, w
			})

			sc := &statConn{Conn: &fakeConn{readN: tt.readN, readErr: tt.readErr}}
			n, err := sc.Read(make([]byte, 16))
			t.Logf("readN=%d readErr=%v → n=%d err=%v reported=%v r=%d", tt.readN, tt.readErr, n, err, called, gotR)

			if n != tt.readN {
				t.Fatalf("n = %d, want %d", n, tt.readN)
			}
			if !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tt.wantErrIs)
			}
			if called != tt.reported {
				t.Fatalf("上报状态 = %v, want %v", called, tt.reported)
			}
			if tt.reported && (gotR != tt.wantR || gotW != 0) {
				t.Fatalf("上报值 = (%d, %d), want (%d, 0)", gotR, gotW, tt.wantR)
			}
		})
	}
}

func TestStatConn_写路径(t *testing.T) {
	wantErr := errors.New("write boom")
	tests := []struct {
		name      string
		writeN    int
		writeErr  error
		wantW     int64
		reported  bool
		wantErrIs error
	}{
		{name: "正常写上报", writeN: 20, wantW: 20, reported: true},
		{name: "写0不上报", writeN: 0, reported: false},
		{name: "有数据且有错也上报", writeN: 7, writeErr: wantErr, wantW: 7, reported: true, wantErrIs: wantErr},
		{name: "0字节带错不上报", writeN: 0, writeErr: wantErr, reported: false, wantErrIs: wantErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetHook(t)
			var gotR, gotW int64
			var called bool
			SetHook(func(r, w int64) {
				called = true
				gotR, gotW = r, w
			})

			sc := &statConn{Conn: &fakeConn{writeN: tt.writeN, writeErr: tt.writeErr}}
			n, err := sc.Write(make([]byte, 16))
			t.Logf("writeN=%d writeErr=%v → n=%d err=%v reported=%v w=%d", tt.writeN, tt.writeErr, n, err, called, gotW)

			if n != tt.writeN {
				t.Fatalf("n = %d, want %d", n, tt.writeN)
			}
			if !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("err = %v, want %v", err, tt.wantErrIs)
			}
			if called != tt.reported {
				t.Fatalf("上报状态 = %v, want %v", called, tt.reported)
			}
			if tt.reported && (gotW != tt.wantW || gotR != 0) {
				t.Fatalf("上报值 = (%d, %d), want (0, %d)", gotR, gotW, tt.wantW)
			}
		})
	}
}

// TestStatConn_Concurrent 验证多 goroutine 并发读写下 hook 累加值正确,
// 同时配合 -race 检测竞态。
func TestStatConn_并发读写累加(t *testing.T) {
	resetHook(t)

	var totalR, totalW int64
	SetHook(func(r, w int64) {
		atomic.AddInt64(&totalR, r)
		atomic.AddInt64(&totalW, w)
	})

	const goroutines, iters = 8, 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := &statConn{Conn: &fakeConn{readN: 3, writeN: 5}}
			buf := make([]byte, 8)
			for j := 0; j < iters; j++ {
				_, _ = sc.Read(buf)
				_, _ = sc.Write(buf)
			}
		}()
	}
	wg.Wait()

	wantR := int64(goroutines * iters * 3)
	wantW := int64(goroutines * iters * 5)
	if totalR != wantR || totalW != wantW {
		t.Fatalf("累加值 = (%d, %d), want (%d, %d)", totalR, totalW, wantR, wantW)
	}
}

// ---------------------------------------------------------------------------
// 第二轮(test-coverage-angles 查漏补缺): 对照角度表补齐现有用例未覆盖的角度。
// ---------------------------------------------------------------------------

// spyConn 记录被透传调用的内嵌方法次数, 用于验证 statConn 只重写 Read/Write,
// 其余方法经 net.Conn 内嵌原样透传到底层。
type spyConn struct {
	net.Conn
	closeCalls    int
	deadlineCalls int
}

func (s *spyConn) Close() error                  { s.closeCalls++; return s.Conn.Close() }
func (s *spyConn) SetDeadline(t time.Time) error { s.deadlineCalls++; return s.Conn.SetDeadline(t) }

// 角度 #7 契约: statConn 仅重写 Read/Write, 内嵌方法须透传, 否则会悄悄吞掉
// 调用方对底层 conn 的 Close/超时控制 —— 这是"只测 Read/Write"最易漏的契约。
func TestStatConn_内嵌方法透传(t *testing.T) {
	resetHook(t)
	SetHook(func(r, w int64) {})

	spy := &spyConn{Conn: &fakeConn{}}
	wrapped := WrapConn(spy)
	if _, ok := wrapped.(*statConn); !ok {
		t.Fatalf("前提: 应包裹成 *statConn, got %T", wrapped)
	}

	if err := wrapped.Close(); err != nil {
		t.Fatalf("Close 透传不应报错: %v", err)
	}
	if err := wrapped.SetDeadline(time.Time{}); err != nil {
		t.Fatalf("SetDeadline 透传不应报错: %v", err)
	}
	if spy.closeCalls != 1 {
		t.Fatalf("Close 应透传到底层 1 次, got %d", spy.closeCalls)
	}
	if spy.deadlineCalls != 1 {
		t.Fatalf("SetDeadline 应透传到底层 1 次, got %d", spy.deadlineCalls)
	}
}

// 角度 #8 状态转换: 重复 SetHook 后者胜, 旧回调不再触发。
func TestSetHook_后者覆盖前者(t *testing.T) {
	resetHook(t)

	var first, second int
	SetHook(func(r, w int64) { first++ })
	SetHook(func(r, w int64) { second++ })

	report(1, 0)
	if first != 0 {
		t.Fatalf("旧回调不应再被触发, got %d", first)
	}
	if second != 1 {
		t.Fatalf("新回调应被触发 1 次, got %d", second)
	}
}

// 角度 #8 状态转换: 已包装的 statConn 在 hook 被关闭后仍能正常读写, 只是不再上报
// —— 验证关闭统计不影响数据通路(零行为变化)。
func TestStatConn_关闭Hook后仍可读不报(t *testing.T) {
	resetHook(t)

	var calls int
	SetHook(func(r, w int64) { calls++ })
	sc := WrapConn(&fakeConn{readN: 5}) // 注入态下完成包装

	SetHook(nil) // 关闭统计

	n, err := sc.Read(make([]byte, 8))
	if n != 5 || err != nil {
		t.Fatalf("关闭统计后仍应正常读: n=%d err=%v", n, err)
	}
	if calls != 0 {
		t.Fatalf("关闭统计后不应再上报, got %d 次", calls)
	}
}

// 角度 #8 状态转换: 关 -> 开重新生效, 仅重新启用后那次上报。
func TestSetHook_关闭后再开只计新的(t *testing.T) {
	resetHook(t)

	var calls int
	SetHook(func(r, w int64) { calls++ })
	SetHook(nil)
	report(1, 0) // 关闭期不上报
	SetHook(func(r, w int64) { calls++ })
	report(1, 0) // 重新启用后上报

	if calls != 1 {
		t.Fatalf("仅重新启用后那次应上报, got %d", calls)
	}
}

// 角度 #5 防御分支: report 的内层 *p != nil 守卫。SetHook 不会存入 nil 函数值,
// 但底层若被存入指向 nil Hook 的指针(防御性场景), report 必须不 panic、不调用。
func TestReport_内层nil函数不panic(t *testing.T) {
	resetHook(t)

	var nilHook Hook     // 函数值为 nil
	hook.Store(&nilHook) // 指针非 nil, 但 *p == nil

	// 不 panic 即覆盖了 `*p != nil` 为 false 的分支。
	report(1, 2)
}

// 角度 #6 并发: atomic.Pointer 的 Store(SetHook) 与 Load(report) 并发, 一边高频读写、
// 一边反复切换回调(含 nil), 仅以 -race 验证无数据竞争、无 panic。切换下累加值不确定,
// 故不断言数值 —— 这正是该用例区别于 TestStatConn_Concurrent(固定回调、断言精确和)之处。
func TestSetHook_并发切换不竞态(t *testing.T) {
	resetHook(t)

	var total int64
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ { // 读写方
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := &statConn{Conn: &fakeConn{readN: 1, writeN: 1}}
			buf := make([]byte, 4)
			for j := 0; j < 300; j++ {
				_, _ = sc.Read(buf)
				_, _ = sc.Write(buf)
			}
		}()
	}
	for i := 0; i < 2; i++ { // 切换方
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				if j%2 == 0 {
					SetHook(func(r, w int64) { atomic.AddInt64(&total, r+w) })
				} else {
					SetHook(nil)
				}
			}
		}()
	}
	wg.Wait()
}

// 角度 #12 对抗输入: 空 buffer 的 Read/Write 不应 panic; 底层返回 0 时不上报。
func TestStatConn_空缓冲不上报(t *testing.T) {
	resetHook(t)

	var calls int
	SetHook(func(r, w int64) { calls++ })

	sc := &statConn{Conn: &fakeConn{readN: 0, writeN: 0}}
	if n, err := sc.Read([]byte{}); n != 0 || err != nil {
		t.Fatalf("空 buffer 读应返回 (0,nil), got (%d,%v)", n, err)
	}
	if n, err := sc.Write([]byte{}); n != 0 || err != nil {
		t.Fatalf("空 buffer 写应返回 (0,nil), got (%d,%v)", n, err)
	}
	if calls != 0 {
		t.Fatalf("0 字节不应上报, got %d 次", calls)
	}
}

func TestWrapConn_重复包装不双计(t *testing.T) {
	resetHook(t)
	var reads, writes atomic.Int64
	SetHook(func(r, w int64) {
		reads.Add(r)
		writes.Add(w)
	})

	c := &fakeConn{readN: 4, writeN: 6}
	first := WrapConn(c)
	second := WrapConn(first)
	t.Logf("first=%T second=%T same=%v", first, second, first == second)
	if first != second {
		t.Fatal("重复 WrapConn 应返回同一连接")
	}

	n, err := second.Read(make([]byte, 8))
	t.Logf("Read n=%d err=%v reads=%d", n, err, reads.Load())
	if n != 4 || err != nil {
		t.Fatalf("Read = (%d,%v), want (4,nil)", n, err)
	}
	n, err = second.Write(make([]byte, 8))
	t.Logf("Write n=%d err=%v writes=%d", n, err, writes.Load())
	if n != 6 || err != nil {
		t.Fatalf("Write = (%d,%v), want (6,nil)", n, err)
	}
	if reads.Load() != 4 || writes.Load() != 6 {
		t.Fatalf("上报 read=%d write=%d, want 4/6（不得双计）", reads.Load(), writes.Load())
	}
}

func TestWrapConn_未注入时返回原连接_事后SetHook不生效(t *testing.T) {
	resetHook(t)
	c := &fakeConn{readN: 3, writeN: 5}
	got := WrapConn(c)
	t.Logf("未注入 WrapConn → %T same=%v", got, got == net.Conn(c))
	if got != net.Conn(c) {
		t.Fatal("hook 未注入时应返回原连接")
	}

	var calls int
	SetHook(func(r, w int64) { calls++ })
	n, err := got.Read(make([]byte, 8))
	t.Logf("事后 SetHook 后 Read n=%d err=%v calls=%d", n, err, calls)
	if n != 3 || err != nil {
		t.Fatalf("原连接仍应可读: n=%d err=%v", n, err)
	}
	if calls != 0 {
		t.Fatalf("事后注入 Hook 不得追溯包装已返回的原连接, calls=%d", calls)
	}
}
