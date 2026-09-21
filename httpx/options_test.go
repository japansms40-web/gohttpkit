package httpx_test

// options_test.go —— Options / RetryPolicy 归一化的单元契约。

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────────────── options.go ───────────────────────────────

func TestRetryPolicy_归一化(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

	t.Run("零值取默认", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
		p := c.RetryPolicy()
		t.Logf("RetryPolicy=%+v", p)
		if p.MaxRetries != 3 || p.BaseBackoff != 200*time.Millisecond || p.MaxBackoff != 0 {
			t.Fatalf("p = %+v", p)
		}
		if p.IsRetryable == nil {
			t.Fatal("IsRetryable 应被补上默认实现")
		}
	})

	t.Run("NoRetry 显式关闭", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}, Retry: httpx.NoRetry()})
		got := c.RetryPolicy().MaxRetries
		t.Logf("MaxRetries=%d", got)
		if got != 0 {
			t.Fatalf("MaxRetries = %d, want 0", got)
		}
	})

	t.Run("环境变量不再覆盖默认", func(t *testing.T) {
		t.Setenv("HTTPKIT_HTTP_MAX_RETRIES", "1")
		t.Setenv("HTTPKIT_HTTP_RETRY_BACKOFF_MS", "5")
		t.Setenv("HTTPKIT_HTTP_RETRY_MAX_BACKOFF_MS", "7")
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: srv.URL}})
		p := c.RetryPolicy()
		t.Logf("RetryPolicy=%+v (env 应被忽略)", p)
		if p.MaxRetries != 3 || p.BaseBackoff != 200*time.Millisecond || p.MaxBackoff != 0 {
			t.Fatalf("p = %+v, want 代码默认值", p)
		}
	})

	t.Run("负重试次数与非法退避被纠正", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: srv.URL},
			Retry:   &httpx.RetryPolicy{MaxRetries: -5, BaseBackoff: -1, IsRetryable: func(error) bool { return true }},
		})
		p := c.RetryPolicy()
		t.Logf("RetryPolicy=%+v", p)
		if p.MaxRetries != 0 {
			t.Fatalf("负重试次数应纠正为 0, got %d", p.MaxRetries)
		}
		if p.BaseBackoff != 200*time.Millisecond {
			t.Fatalf("非法退避应回落默认, got %v", p.BaseBackoff)
		}
	})

	t.Run("WithRetry 全字段生效", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: srv.URL},
			Retry:   httpx.WithRetry(2, 3*time.Millisecond, 4*time.Millisecond),
		})
		p := c.RetryPolicy()
		t.Logf("RetryPolicy=%+v", p)
		if p.MaxRetries != 2 || p.BaseBackoff != 3*time.Millisecond || p.MaxBackoff != 4*time.Millisecond {
			t.Fatalf("p = %+v", p)
		}
	})
}

func TestOptions_超时默认值(t *testing.T) {
	t.Run("默认 30s", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
		t.Logf("timeout=%v", c.HTTPClient.Timeout)
		if c.HTTPClient.Timeout != 30*time.Second {
			t.Fatalf("timeout = %v", c.HTTPClient.Timeout)
		}
	})
	t.Run("显式值生效", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}, Timeout: 9 * time.Second})
		t.Logf("timeout=%v", c.HTTPClient.Timeout)
		if c.HTTPClient.Timeout != 9*time.Second {
			t.Fatalf("timeout = %v", c.HTTPClient.Timeout)
		}
	})
	t.Run("环境变量不再覆盖超时", func(t *testing.T) {
		t.Setenv("HTTPKIT_HTTP_TIMEOUT_MS", "1500")
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
		t.Logf("timeout=%v (env 应被忽略)", c.HTTPClient.Timeout)
		if c.HTTPClient.Timeout != 30*time.Second {
			t.Fatalf("timeout = %v, want 30s 代码默认", c.HTTPClient.Timeout)
		}
	})
}

func TestOptions_ResponseHeaderTimeout(t *testing.T) {
	t.Run("零值回落 15s", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}})
		got := mustTransport(t, c).ResponseHeaderTimeout
		t.Logf("ResponseHeaderTimeout=%v", got)
		if got != 15*time.Second {
			t.Fatalf("ResponseHeaderTimeout = %v, want 15s", got)
		}
	})
	t.Run("显式值生效", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{
			Headers:               httpx.StaticHeaders{Base: "https://x.example"},
			ResponseHeaderTimeout: 3 * time.Second,
		})
		got := mustTransport(t, c).ResponseHeaderTimeout
		t.Logf("ResponseHeaderTimeout=%v", got)
		if got != 3*time.Second {
			t.Fatalf("ResponseHeaderTimeout = %v, want 3s", got)
		}
	})
	t.Run("负数回落 15s", func(t *testing.T) {
		c := newClientWith(t, httpx.Options{
			Headers:               httpx.StaticHeaders{Base: "https://x.example"},
			ResponseHeaderTimeout: -time.Second,
		})
		got := mustTransport(t, c).ResponseHeaderTimeout
		t.Logf("ResponseHeaderTimeout=%v", got)
		if got != 15*time.Second {
			t.Fatalf("ResponseHeaderTimeout = %v, want 15s", got)
		}
	})
	t.Run("自带 Transport 时不改写", func(t *testing.T) {
		custom := &http.Transport{ResponseHeaderTimeout: 99 * time.Second}
		c := newClientWith(t, httpx.Options{
			Headers:               httpx.StaticHeaders{Base: "https://x.example"},
			Transport:             custom,
			ResponseHeaderTimeout: 3 * time.Second,
		})
		t.Logf("custom RHT=%v still=%v", custom.ResponseHeaderTimeout, c.HTTPClient.Transport == custom)
		if c.HTTPClient.Transport != custom {
			t.Fatal("应原样使用调用方的 Transport")
		}
		if custom.ResponseHeaderTimeout != 99*time.Second {
			t.Fatalf("不该改写调用方 Transport: %v", custom.ResponseHeaderTimeout)
		}
	})
}

func TestOptions_自带Transport则不再调优也不接代理(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 999}
	c := newClientWith(t, httpx.Options{
		Headers:   httpx.StaticHeaders{Base: "https://x.example"},
		Transport: custom,
		ProxyURL:  "ftp://bad:1", // 非法代理也不该报错——因为根本不会去接
	})
	t.Logf("custom MaxIdleConns=%d same=%v", custom.MaxIdleConns, c.HTTPClient.Transport == custom)
	if c.HTTPClient.Transport != custom {
		t.Fatal("应原样使用调用方的 Transport")
	}
}

func TestOptions_DisableOriginReferer(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.DisableOriginReferer = true })
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	h := srv.last().Header
	t.Logf("origin=%q referer=%q", h.Get("origin"), h.Get("referer"))
	if h.Get("origin") != "" || h.Get("referer") != "" {
		t.Fatalf("关闭后不该注入 origin/referer: %v", h)
	}
}

func mustTransport(t *testing.T, c *httpx.Client) *http.Transport {
	t.Helper()
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型 = %T, want *http.Transport", c.HTTPClient.Transport)
	}
	return tr
}
