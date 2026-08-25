package logger

import (
	"context"
	"log/slog"
	"testing"
)

// TestParseLevel 各级别字符串解析,未知值/大小写/空串统一兜底 info
// 角度: #11 输入多样性 + #7 契约(parseLevel 文档承诺非法值回退 info)
func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},        // 空串兜底
		{"DEBUG", slog.LevelInfo},   // 大小写敏感,非全小写兜底
		{"verbose", slog.LevelInfo}, // 未知值兜底
		{" warn ", slog.LevelInfo},  // 带空白不做 trim,兜底
	}
	for _, c := range cases {
		if got := parseLevel(c.in); got != c.want {
			t.Errorf("parseLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestLevelFunctions 四个级别函数各自输出对应 level,且 msg/attr 正常透传
// 角度: #1 happy path + #9 side effect(断言写出的 level 而非返回值) + #11 输入多样性
func TestLevelFunctions(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{Level: slog.LevelDebug})
	cases := []struct {
		name  string
		fn    func(context.Context, string, ...slog.Attr)
		level string
	}{
		{"Debug", Debug, "DEBUG"},
		{"Info", Info, "INFO"},
		{"Warn", Warn, "WARN"},
		{"Error", Error, "ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf.Reset()
			c.fn(context.Background(), "the-msg", slog.Int("n", 7))
			m := lastLine(t, buf)
			if m["level"] != c.level {
				t.Errorf("level = %v, want %v", m["level"], c.level)
			}
			if m["msg"] != "the-msg" {
				t.Errorf("msg = %v, want the-msg", m["msg"])
			}
			if m["n"] != float64(7) { // JSON 数字解析为 float64
				t.Errorf("attr n = %v, want 7", m["n"])
			}
		})
	}
}

// TestLevelFiltering 低于阈值的日志被 Enabled 拦截、不进入输出;达标的正常写出
// 角度: #9 side effect + #8 状态(阈值) —— 覆盖 log() 的 Enabled 短路分支
func TestLevelFiltering(t *testing.T) {
	buf := captureJSON(t, &slog.HandlerOptions{Level: slog.LevelWarn})

	Debug(context.Background(), "dropped-debug")
	Info(context.Background(), "dropped-info")
	if buf.Len() != 0 {
		t.Fatalf("低于 warn 的日志不应输出: %s", buf.String())
	}

	Warn(context.Background(), "kept-warn")
	if m := lastLine(t, buf); m["msg"] != "kept-warn" {
		t.Fatalf("warn 级别应输出: %v", m)
	}
}

// TestNilContext ctx 为 nil 时回退 context.Background(),不 panic 且正常输出
// 角度: #5 nil 值
func TestNilContext(t *testing.T) {
	buf := captureJSON(t, nil)
	var nilCtx context.Context // 显式 nil,避免 vet 对字面 nil 的告警
	Info(nilCtx, "nil-ctx-ok")
	if m := lastLine(t, buf); m["msg"] != "nil-ctx-ok" {
		t.Fatalf("nil ctx 应正常输出: %v", m)
	}
}

// TestNamedErrNonNil NamedErr 以自定义键名输出错误信息(nil 安全已由 TestErrNilSafe 覆盖)
// 角度: #11 输入多样性
func TestNamedErrNonNil(t *testing.T) {
	buf := captureJSON(t, nil)
	Info(context.Background(), "named", NamedErr("classify_err", context.Canceled))
	if m := lastLine(t, buf); m["classify_err"] != context.Canceled.Error() {
		t.Fatalf("classify_err = %v, want %v", m["classify_err"], context.Canceled.Error())
	}
}
