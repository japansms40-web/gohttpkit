package loggertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/japansms40-web/gohttpkit/logger"
)

// capture.go —— 测试用的日志捕获：把 logger.Init 的控制台一路接到内存缓冲区，再按行解析。
// 独立成包：生产代码不该看到测试钩子，logger 包自己也只保留 Init 这一个入口。

// Record 捕获到的一条日志。
type Record struct {
	// Level 级别文本：DEBUG、INFO、WARN、ERROR。不是 JSON 的行为空串。
	Level string
	// Msg 日志消息；事件日志里与 event 字段同值。不是 JSON 的行是整行原文。
	Msg string
	// Attrs time、level、msg、source 之外的全部字段。数值按 JSON 规则是 float64。不是 JSON 的行为 nil。
	Attrs map[string]any
}

// Event 返回这条记录的事件名。
// 输入：无。
// 返回：event 字段的字符串值；没有这个字段或类型不是字符串时为空串。
func (r Record) Event() string {
	name, _ := r.Attrs[logger.FieldEvent].(string)
	return name
}

// Text 返回字段 key 的文本形式，省掉用例里对 JSON 数值类型的断言转换。
// 输入 key：字段名。
// 返回：没有这个字段时为空串；字符串原样；数值按最短十进制写法（JSON 里的 42 得到 "42"，不是 "4.2e+01"）；其余类型按 fmt.Sprint。
// 例：日志带 slog.Int("n", 7) → Text("n") == "7"；带 slog.Bool("ok", true) → Text("ok") == "true"。
func (r Record) Text(key string) string {
	switch v := r.Attrs[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// Recorder 收集 Capture 期间的全部日志输出。
// 并发：Write 与读取方法都拿 mu，可被任意 goroutine 同时调用。
type Recorder struct {
	// mu 保护 buf。
	mu sync.Mutex
	// buf 到目前为止的原始输出，每条日志一行 JSON。
	buf bytes.Buffer
}

// Write 追加一段日志输出，实现 io.Writer。
// 输入 p：一次写入的字节，不会被保留引用。
// 返回：写入的字节数与 nil；内存缓冲区不会写失败。
func (r *Recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

// String 返回到目前为止的原始输出。
// 输入：无。
// 返回：全部已写入内容的副本，每条日志一行。
func (r *Recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

// Records 把到目前为止的输出逐行解析成记录，顺序与写入顺序一致。
// 输入：无。
// 返回：每个非空行一条 Record；没有输出时为 nil。不是 JSON 的行不丢弃，整行放进 Msg，方便用例发现裸文本输出。
func (r *Recorder) Records() []Record {
	var out []Record
	for line := range strings.SplitSeq(r.String(), "\n") {
		if line == "" {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			out = append(out, Record{Msg: line})
			continue
		}
		rec := Record{Attrs: fields}
		rec.Level, _ = fields[slog.LevelKey].(string)
		rec.Msg, _ = fields[slog.MessageKey].(string)
		for _, key := range []string{slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey} {
			delete(fields, key)
		}
		out = append(out, rec)
	}
	return out
}

// Events 返回到目前为止每条事件日志的事件名，顺序与写入顺序一致。
// 输入：无。
// 返回：各记录的 event 字段；没有 event 字段的记录跳过。没有事件日志时为 nil。
func (r *Recorder) Events() []string {
	var names []string
	for _, rec := range r.Records() {
		if name := rec.Event(); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// Capture 把进程日志接进内存，用例结束时还原。
// 输入 tb：当前用例，用来登记收尾。level：捕获的最低级别，取 logger.Level* 常量。attrs：每条日志都带的固定字段，可为空。
// 返回：非 nil *Recorder，之后打出的日志都能从它读到。
// 例：rec := loggertest.Capture(t, logger.LevelDebug); ...; rec.Events()。
// 它调的是 logger.Init，所以标准库 log 与 slog 默认 logger 的输出也会进来；收尾调 logger.Close。
func Capture(tb testing.TB, level logger.Level, attrs ...slog.Attr) *Recorder {
	tb.Helper()
	r := &Recorder{}
	// 只写内存、不落盘，Init 没有会失败的步骤。
	_ = logger.Init(logger.Config{Level: level, Format: logger.FormatJSON, Output: logger.OutputConsole, Writer: r}, attrs...)
	tb.Cleanup(logger.Close)
	return r
}
