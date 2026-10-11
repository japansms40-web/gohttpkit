package loggertest

import (
	stdlog "log"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/japansms40-web/gohttpkit/logger"
)

func TestCapture_记录级别消息字段与事件(t *testing.T) {
	rec := Capture(t, logger.LevelDebug, slog.String("app", "svc"))

	ctx := logger.WithTraceID(t.Context(), "trace-cap")
	logger.InfoEvent(ctx, logger.NewEvent("account.sync.started"), slog.Int("n", 7))
	logger.Warn(ctx, "free-text")

	recs := rec.Records()
	t.Logf("records=%+v", recs)
	if len(recs) != 2 {
		t.Fatalf("应捕获 2 条，实际 %d 条: %s", len(recs), rec.String())
	}
	first := recs[0]
	if first.Level != "INFO" || first.Msg != "account.sync.started" || first.Event() != "account.sync.started" {
		t.Fatalf("第一条 = %+v", first)
	}
	if first.Attrs["n"] != float64(7) || first.Attrs["app"] != "svc" || first.Attrs["trace_id"] != "trace-cap" {
		t.Fatalf("第一条字段 = %v", first.Attrs)
	}
	for _, key := range []string{slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey} {
		if _, ok := first.Attrs[key]; ok {
			t.Fatalf("Attrs 不应再含 %q: %v", key, first.Attrs)
		}
	}
	if recs[1].Level != "WARN" || recs[1].Msg != "free-text" || recs[1].Event() != "" {
		t.Fatalf("第二条 = %+v", recs[1])
	}
	if got := rec.Events(); !slices.Equal(got, []string{"account.sync.started"}) {
		t.Fatalf("Events() = %v", got)
	}
}

func TestCapture_低于级别的不记录(t *testing.T) {
	rec := Capture(t, logger.LevelWarn)

	logger.Info(t.Context(), "dropped")
	logger.Error(t.Context(), "kept")

	recs := rec.Records()
	t.Logf("records=%+v", recs)
	if len(recs) != 1 || recs[0].Msg != "kept" || recs[0].Level != "ERROR" {
		t.Fatalf("只应记录 error: %+v", recs)
	}
}

func TestCapture_标准库log也进来(t *testing.T) {
	rec := Capture(t, logger.LevelInfo)

	stdlog.Print("via-std-log")

	recs := rec.Records()
	t.Logf("records=%+v", recs)
	if len(recs) != 1 || recs[0].Msg != "via-std-log" || recs[0].Level != "INFO" {
		t.Fatalf("标准库 log 应并入捕获: %+v", recs)
	}
}

func TestCapture_并发写不丢行(t *testing.T) {
	rec := Capture(t, logger.LevelInfo)

	const workers, perWorker = 50, 20
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range perWorker {
				logger.Info(t.Context(), "concurrent", slog.Int("w", w), slog.Int("i", i))
			}
		})
	}
	wg.Wait()

	recs := rec.Records()
	t.Logf("并发 %d×%d 条，捕获 %d 条", workers, perWorker, len(recs))
	if len(recs) != workers*perWorker {
		t.Fatalf("应捕获 %d 条，实际 %d 条", workers*perWorker, len(recs))
	}
	for _, r := range recs {
		if r.Msg != "concurrent" {
			t.Fatalf("出现了被写坏的行: %+v", r)
		}
	}
}

func TestCapture_用例结束后还原(t *testing.T) {
	before := logger.Enabled(t.Context(), slog.LevelDebug)
	var inner *Recorder
	t.Run("捕获期间", func(t *testing.T) {
		inner = Capture(t, logger.LevelDebug)
		if !logger.Enabled(t.Context(), slog.LevelDebug) {
			t.Fatal("捕获期间 debug 应已开启")
		}
	})

	after := logger.Enabled(t.Context(), slog.LevelDebug)
	t.Logf("before=%v after=%v", before, after)
	if after != before {
		t.Fatalf("子用例结束后应还原到捕获之前: before=%v after=%v", before, after)
	}
	logger.Error(t.Context(), "after-capture")
	if got := inner.Records(); len(got) != 0 {
		t.Fatalf("还原之后的日志不应再进旧 Recorder: %+v", got)
	}
}

func TestRecorder_非JSON行原样保留且空输出为nil(t *testing.T) {
	r := &Recorder{}
	if r.Records() != nil || r.Events() != nil || r.String() != "" {
		t.Fatalf("没有输出时应为空: records=%v events=%v raw=%q", r.Records(), r.Events(), r.String())
	}

	n, err := r.Write([]byte("plain text line\n{\"level\":\"INFO\",\"msg\":\"ok\",\"event\":7}\n"))
	if err != nil || n == 0 {
		t.Fatalf("Write n=%d err=%v", n, err)
	}

	recs := r.Records()
	t.Logf("records=%+v", recs)
	if len(recs) != 2 {
		t.Fatalf("应有 2 条: %+v", recs)
	}
	if recs[0].Msg != "plain text line" || recs[0].Level != "" || recs[0].Attrs != nil {
		t.Fatalf("不是 JSON 的行应整行放进 Msg: %+v", recs[0])
	}
	if recs[1].Msg != "ok" || recs[1].Event() != "" {
		t.Fatalf("event 不是字符串时事件名应为空: %+v", recs[1])
	}
	if r.Events() != nil {
		t.Fatalf("没有字符串 event 时 Events 应为 nil: %v", r.Events())
	}
}

func TestRecord_Text按类型给出文本(t *testing.T) {
	rec := Capture(t, logger.LevelInfo)

	logger.Info(t.Context(), "typed", slog.String("s", "str"), slog.Int("n", 42), slog.Float64("f", 1.5), slog.Bool("ok", true), slog.Int64("big", 1234567890123))

	recs := rec.Records()
	if len(recs) != 1 {
		t.Fatalf("应捕获 1 条: %+v", recs)
	}
	got := recs[0]
	cases := map[string]string{"s": "str", "n": "42", "f": "1.5", "ok": "true", "big": "1234567890123", "missing": ""}
	for key, want := range cases {
		t.Logf("Text(%q) = %q", key, got.Text(key))
		if got.Text(key) != want {
			t.Errorf("Text(%q) = %q, want %q", key, got.Text(key), want)
		}
	}
	if (Record{}).Text("any") != "" {
		t.Error("零值 Record 的任何字段都应是空串")
	}
}
