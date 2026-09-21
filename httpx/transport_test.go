package httpx_test

// transport_test.go —— NewTransport 调优参数与代理失败。

import (
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────────────── transport.go ───────────────────────────────

func TestNewTransport_调优参数与代理错误(t *testing.T) {
	tr, err := httpx.NewTransport("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("DisableCompression=%v ForceHTTP2=%v RHT=%v err=%v", tr.DisableCompression, tr.ForceAttemptHTTP2, tr.ResponseHeaderTimeout, err)
	if !tr.DisableCompression {
		t.Fatal("必须关掉标准库自动 gzip，否则与自己的解压层重复")
	}
	if !tr.ForceAttemptHTTP2 || tr.TLSClientConfig == nil {
		t.Fatalf("调优参数缺失: %+v", tr)
	}
	if tr.ResponseHeaderTimeout != 15*time.Second {
		t.Fatalf("NewTransport 默认 ResponseHeaderTimeout = %v, want 15s", tr.ResponseHeaderTimeout)
	}
	_, err = httpx.NewTransport("ftp://x:1")
	t.Logf("ftp proxy err=%v", err)
	if err == nil {
		t.Fatal("非法代理应报错")
	}
}
