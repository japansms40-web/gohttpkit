package interceptor_test

// body_decode_more_test.go —— 补解压角度：四种 content-encoding 的 roundtrip、
// 损坏流报错，以及响应头 hook / 请求改写器的非空路径。

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	stderrors "errors"
	"net/http"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func gzipBytes(t *testing.T, p []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write(p)
	_ = w.Close()
	return b.Bytes()
}

func TestBodyDecode_四种编码roundtrip(t *testing.T) {
	payload := []byte(`{"msg":"你好 world","n":42}`)
	enc := map[string]func() []byte{
		"gzip": func() []byte { return gzipBytes(t, payload) },
		"deflate": func() []byte {
			var b bytes.Buffer
			w, _ := flate.NewWriter(&b, flate.DefaultCompression)
			_, _ = w.Write(payload)
			_ = w.Close()
			return b.Bytes()
		},
		"br": func() []byte {
			var b bytes.Buffer
			w := brotli.NewWriter(&b)
			_, _ = w.Write(payload)
			_ = w.Close()
			return b.Bytes()
		},
		"zstd": func() []byte {
			var b bytes.Buffer
			w, _ := zstd.NewWriter(&b)
			_, _ = w.Write(payload)
			_ = w.Close()
			return b.Bytes()
		},
	}
	for name, mk := range enc {
		body := mk()
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-encoding", name)
			_, _ = w.Write(body)
		})
		c := newClient(t, srv.Server, func(o *httpx.Options) { o.Interceptors = interceptor.DefaultChain() })
		got, err := c.Get(context.Background(), "/x", nil)
		t.Logf("encoding=%s → 解压 %d 字节 err=%v", name, len(got), err)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("%s: 解压结果=%q，应为原文", name, got)
		}
	}
}

func TestBodyDecode_损坏gzip报ContentEncodingError(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-encoding", "gzip")
		_, _ = w.Write([]byte("这不是合法的 gzip 流")) // 头就非法，gzip.NewReader 立即失败
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.Interceptors = interceptor.DefaultChain() })
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("损坏 gzip → err=%v", err)
	var ce *httpx.ContentEncodingError
	if !stderrors.As(err, &ce) {
		t.Fatalf("err=%v，应为 *ContentEncodingError", err)
	}
	if ce.Encoding != httpx.EncodingGzip {
		t.Fatalf("Encoding=%q，应为 gzip", ce.Encoding)
	}
}

func TestResponseHeaderCache_OnResponseHeaders回调(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-new-token", "abc123")
		_, _ = w.Write([]byte("ok"))
	})
	var gotToken string
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.DefaultChain()
		o.OnResponseHeaders = func(_ context.Context, h http.Header) { gotToken = h.Get("x-new-token") }
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("hook 收到 x-new-token=%q", gotToken)
	if gotToken != "abc123" {
		t.Fatalf("hook 未被调用或未拿到头，gotToken=%q", gotToken)
	}
}

func TestRequestMutator_非nil改写生效(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.SpliceBeforeTerminal(interceptor.DefaultChain(),
			interceptor.NewRequestMutatorInterceptor(func(req *httpx.Request) {
				req.HTTPReq.Header.Set("x-mutated", "1")
			}))
	})
	if _, err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	srv.mu <- struct{}{}
	t.Logf("服务器收到 x-mutated=%q", last.Header.Get("x-mutated"))
	if last.Header.Get("x-mutated") != "1" {
		t.Fatalf("请求改写器未生效，x-mutated=%q", last.Header.Get("x-mutated"))
	}
}
