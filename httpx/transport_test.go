package httpx

// transport_test.go —— NewTransport 调优参数与代理失败。

import (
	"errors"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/netproxy"
)

// ─────────────────────────────── transport.go ───────────────────────────────

func TestNewTransport_调优参数与代理错误(t *testing.T) {
	tr, err := NewTransport("")
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
	_, err = NewTransport("ftp://x:1")
	t.Logf("ftp proxy err=%v", err)
	if err == nil {
		t.Fatal("非法代理应报错")
	}
}

func TestNewTransport_ftp是UnsupportedScheme(t *testing.T) {
	_, err := NewTransport("ftp://h:1")
	t.Logf("ftp → %v", err)
	var ue *netproxy.UnsupportedProxySchemeError
	if !errors.As(err, &ue) || ue.Scheme != "ftp" {
		t.Fatalf("要 *UnsupportedProxySchemeError{ftp}，got %v", err)
	}
}

func TestNewTransport_headerTimeout非正回落15s(t *testing.T) {
	for _, ht := range []time.Duration{0, -time.Second} {
		tr, err := newTransport("", ht)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("headerTimeout=%v → ResponseHeaderTimeout=%v", ht, tr.ResponseHeaderTimeout)
		if tr.ResponseHeaderTimeout != defaultResponseHeaderTimeout {
			t.Fatalf("应回落 %v，得到 %v", defaultResponseHeaderTimeout, tr.ResponseHeaderTimeout)
		}
	}
}
