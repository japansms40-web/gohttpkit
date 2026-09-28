package interceptor_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

// ─────────────────────────── 观察层与核心层的剩余分支 ───────────────────────────

func TestLogging_慢请求降级为warn(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	})
	// 全量与摘要两种日志形态都走一遍 slow 分支
	for _, summary := range []bool{false, true} {
		c := newClient(t, srv.Server, func(o *httpx.Options) {
			o.LogSummaryOnly = summary
			o.SlowMS = time.Millisecond
		})
		if _, err := c.Get(t.Context(), "/x", nil); err != nil {
			t.Fatal(err)
		}
		t.Logf("summary=%v slowMS=%v", summary, time.Millisecond)
	}
}

func TestLogging_摘要模式下4xx仍打全量(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("nope"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.LogSummaryOnly = true })
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("summary 4xx status=%d", c.SnapshotResponseStatusCode())
	if c.SnapshotResponseStatusCode() != 404 {
		t.Fatal("状态码没记上")
	}
}

func TestLogging_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewLoggingInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("logging inner err=%v is=%v", err, errors.Is(err, sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestLogging_摘要2xx字段可被抓到(t *testing.T) {
	buf := captureInterceptorLogs(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.LogSummaryOnly = true })
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("logs=%s", buf.String())
	var sawTxn bool
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec["event"] == "http.transaction" {
			sawTxn = true
		}
	}
	if !sawTxn {
		t.Fatal("摘要 2xx 仍应打 event=http.transaction")
	}
}
