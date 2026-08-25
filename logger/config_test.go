package logger

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestSetLogger 注入非 nil *slog.Logger 生效,且其 handler 被包装后自动获得 trace_id
// 角度: #8 状态转换 —— 覆盖 SetLogger 的非 nil 分支
func TestSetLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	SetLogger(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })

	Info(WithTraceID(context.Background(), "lg"), "via-setlogger")
	m := lastLine(t, buf)
	if m["msg"] != "via-setlogger" {
		t.Fatalf("SetLogger 注入未生效: %v", m)
	}
	if m["trace_id"] != "lg" {
		t.Fatalf("SetLogger 注入的 handler 未自动获得 trace_id: %v", m)
	}
}

// TestSetHandlerNil 传 nil 还原默认:清空 external,注入的 buf 不再被写入
// 角度: #5 nil 值 + #8 状态转换 —— 覆盖 SetHandler 的 nil 分支
func TestSetHandlerNil(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	SetHandler(nil) // 还原默认
	if external.Load() != nil {
		t.Fatal("SetHandler(nil) 应清空 external")
	}

	buf.Reset()
	Info(context.Background(), "after-nil")
	if buf.Len() != 0 {
		t.Fatalf("SetHandler(nil) 后不应再写入旧注入 buf: %s", buf.String())
	}
}

// TestDefaultReflectsActive Default() 返回当前生效 logger,可直接交给其它库使用
// 角度: #8 状态转换 —— 覆盖 Default()
func TestDefaultReflectsActive(t *testing.T) {
	buf := &bytes.Buffer{}
	SetHandler(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { SetLogger(nil) })

	Default().Info("via-default")
	if !strings.Contains(buf.String(), "via-default") {
		t.Fatalf("Default() 未反映已注入的 handler: %s", buf.String())
	}
}

// TestActiveLazyInit 无 external 且 fallback 为 nil 时,active() 惰性创建默认并回填 fallback
// 角度: #8 状态转换(初始化前/后) —— 覆盖 active() 的 CAS 惰性初始化分支
func TestActiveLazyInit(t *testing.T) {
	prevExt := external.Load()
	prevFb := fallback.Load()
	t.Cleanup(func() {
		external.Store(prevExt)
		fallback.Store(prevFb)
	})

	external.Store(nil)
	fallback.Store(nil)

	if active() == nil {
		t.Fatal("惰性初始化应返回非 nil logger")
	}
	if fallback.Load() == nil {
		t.Fatal("惰性初始化应回填 fallback")
	}
}

// TestActiveLazyInitRace fallback 为 nil 时多 goroutine 同时 active():仅一个 CAS 成功,
// 其余落到 fallback.Load() 兜底分支,且全员拿到同一实例(惰性初始化幂等)
// 角度: #6 并发 —— 覆盖 active() CAS 抢输后的 fallback.Load() 分支
func TestActiveLazyInitRace(t *testing.T) {
	prevExt := external.Load()
	prevFb := fallback.Load()
	t.Cleanup(func() {
		external.Store(prevExt)
		fallback.Store(prevFb)
	})

	external.Store(nil)

	// 多轮:每轮把 fallback 归零后齐发,反复制造 CAS 抢输窗口,确保兜底分支至少执行一次
	const rounds, n = 200, 8
	for r := 0; r < rounds; r++ {
		fallback.Store(nil)

		var wg sync.WaitGroup
		start := make(chan struct{})
		got := make([]*slog.Logger, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start // barrier:让所有 goroutine 同时冲 CAS
				got[idx] = active()
			}(i)
		}
		close(start)
		wg.Wait()

		first := got[0]
		if first == nil {
			t.Fatal("惰性初始化返回 nil")
		}
		for i, l := range got {
			if l != first { // 幂等:无论谁赢 CAS,全员拿到同一实例
				t.Fatalf("round %d 并发惰性初始化返回不同实例: got[%d]=%p first=%p", r, i, l, first)
			}
		}
	}
}

// TestFileOutput output=file 写入指定文件,且不存在的父目录被自动创建
// 角度: #10 资源生命周期 —— 覆盖 newDefault 的 file 分支与 mustFileWriter 主路径
func TestFileOutput(t *testing.T) {
	prevFb := fallback.Load()
	t.Cleanup(func() { fallback.Store(prevFb); SetLogger(nil) })

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "app.log") // nested 不存在,须自动创建

	SetLogger(nil) // 确保走 fallback
	SetConfig(Config{Level: "info", Format: "json", Output: "file", FilePath: path})

	Info(context.Background(), "to-file-unique-xyz")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读日志文件失败(目录或文件未创建): %v", err)
	}
	if !strings.Contains(string(data), "to-file-unique-xyz") {
		t.Fatalf("日志未写入文件: %s", data)
	}
}

// TestBothOutput output=both 同时写 stdout 与文件(console 格式)
// 角度: #9 side effect(两个 sink 都要写到) + #8 状态 —— 覆盖 newDefault 的 both/console 分支
func TestBothOutput(t *testing.T) {
	prevFb := fallback.Load()
	origStdout := os.Stdout
	t.Cleanup(func() { fallback.Store(prevFb); os.Stdout = origStdout; SetLogger(nil) })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w // newDefault 在 SetConfig 时按当前 os.Stdout 构造 MultiWriter

	dir := t.TempDir()
	path := filepath.Join(dir, "both.log")

	SetLogger(nil)
	SetConfig(Config{Level: "info", Format: "console", Output: "both", FilePath: path})
	Info(context.Background(), "both-unique-abc")

	os.Stdout = origStdout
	_ = w.Close()
	stdoutData, _ := io.ReadAll(r)

	if !strings.Contains(string(stdoutData), "both-unique-abc") {
		t.Fatalf("both 模式未写入 stdout: %s", stdoutData)
	}
	fileData, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(fileData), "both-unique-abc") {
		t.Fatalf("both 模式未写入文件: err=%v data=%s", err, fileData)
	}
}

// TestMustFileWriterEmptyPath 空路径回退默认 ./logs/app.log,并自动建目录
// 角度: #2 边界(空输入) + #10 资源生命周期
func TestMustFileWriterEmptyPath(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if mustFileWriter("") == nil {
		t.Fatal("空路径应返回默认文件 writer")
	}
	if _, err := os.Stat(filepath.Join(dir, "logs", "app.log")); err != nil {
		t.Fatalf("默认路径文件未创建: %v", err)
	}
}

// TestMustFileWriterPanicsOnBadDir 父路径中存在同名普通文件导致建目录失败 → panic
// 角度: #3 错误路径 + #12 对抗性输入 —— 覆盖 MkdirAll 失败的 panic 分支
func TestMustFileWriterPanicsOnBadDir(t *testing.T) {
	dir := t.TempDir()
	notDir := filepath.Join(dir, "afile")
	if err := os.WriteFile(notDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(notDir, "sub", "app.log") // afile 是文件,不能当目录

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("建目录失败应 panic")
		}
	}()
	mustFileWriter(bad)
}

// TestMustFileWriterPanicsOnOpenFail 目录可建但目标本身是目录,OpenFile 失败 → panic
// 角度: #3 错误路径 —— 覆盖 OpenFile 失败的 panic 分支
func TestMustFileWriterPanicsOnOpenFail(t *testing.T) {
	dir := t.TempDir() // 把目录当文件路径打开必失败

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("打开目录作文件应 panic")
		}
	}()
	mustFileWriter(dir)
}

// TestWithAttrsEmpty 空 attrs 原样返回父 ctx(不分配、不破坏链路)
// 角度: #2 边界 + #5 nil/zero —— 覆盖 WithAttrs 的 len==0 早返回分支
func TestWithAttrsEmpty(t *testing.T) {
	parent := WithAttrs(context.Background(), slog.String("a", "1"))
	if got := WithAttrs(parent); got != parent {
		t.Fatal("空 attrs 应原样返回同一父 ctx")
	}

	base := context.Background()
	if got := WithAttrs(base); got != base {
		t.Fatal("无 attrs 应返回原 ctx")
	}
}

// TestConcurrentContextDerive 并发派生 ctx(WithAttrs copy-on-write + WithTraceID)无数据竞争
// 角度: #6 并发 —— 须配合 -race 运行;共享父 ctx 的 attrs 切片只读、派生只追加副本
func TestConcurrentContextDerive(t *testing.T) {
	buf := captureJSON(t, nil) // slog handler 内部带锁,可并发写
	base := WithAttrs(context.Background(), slog.String("base", "0"))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ctx := WithTraceID(WithAttrs(base, slog.Int("g", n)), "x")
				Info(ctx, "c")
			}
		}(i)
	}
	wg.Wait()
	_ = buf
}
