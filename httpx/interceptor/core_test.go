package interceptor_test

// interceptors_core_test.go —— 核心拦截器（bridge / decode / 终端）的单元契约。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestBodyDecode_Raw为nil时原样放过(t *testing.T) {
	// 回放层 / 自定义终端可能已经把 Body 填好了，此时解压层无事可做。
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewBodyDecodeInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				return &httpx.Response{StatusCode: 200, Header: http.Header{}, Body: []byte("replayed")}, nil
			}),
		},
	})
	body, err := c.Get(context.Background(), "/x", nil)
	t.Logf("raw=nil body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "replayed" {
		t.Fatalf("body = %q", body)
	}
}

func TestBodyDecode_压缩流损坏时报错(t *testing.T) {
	t.Run("gzip", func(t *testing.T) {
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-encoding", "gzip")
			_, _ = w.Write([]byte("this is not compressed at all"))
		})
		c := newClient(t, srv.Server, nil)
		_, err := c.Get(context.Background(), "/x", nil)
		var ce *httpx.ContentEncodingError
		if !errors.As(err, &ce) || ce.Encoding != httpx.EncodingGzip {
			t.Fatalf("err = %v (%T), want *ContentEncodingError Encoding=gzip", err, err)
		}
		t.Logf("gzip construct err Encoding=%q unwrap=%v", ce.Encoding, ce.Err)
	})
	t.Run("zstd", func(t *testing.T) {
		srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-encoding", "zstd")
			_, _ = w.Write([]byte("this is not compressed at all"))
		})
		c := newClient(t, srv.Server, nil)
		_, err := c.Get(context.Background(), "/x", nil)
		if err == nil {
			t.Fatal("zstd 流损坏应报错")
		}
		var ce *httpx.ContentEncodingError
		var re *httpx.ReadResponseBodyError
		if errors.As(err, &ce) {
			if ce.Encoding != httpx.EncodingZstd {
				t.Fatalf("Encoding = %q, want zstd", ce.Encoding)
			}
			t.Logf("zstd construct err Encoding=%q unwrap=%v", ce.Encoding, ce.Err)
			return
		}
		if !errors.As(err, &re) || re.Encoding != httpx.EncodingZstd {
			t.Fatalf("err = %v (%T), want ContentEncodingError 或 ReadResponseBodyError Encoding=zstd", err, err)
		}
		t.Logf("zstd read err Encoding=%q unwrap=%v", re.Encoding, re.Err)
	})
}

func TestBridge_创建请求失败(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, nil)
	_, err := c.Do(context.Background(), httpx.RequestSpec{Method: "BAD METHOD", Path: "/x"})
	var cre *httpx.CreateHTTPRequestError
	if !errors.As(err, &cre) || cre.Method != "BAD METHOD" || cre.Err == nil {
		t.Fatalf("err = %v (%T), want *CreateHTTPRequestError Method=BAD METHOD", err, err)
	}
	t.Logf("CreateHTTPRequestError Method=%q unwrap=%v", cre.Method, cre.Err)
	if srv.count() != 0 {
		t.Fatal("请求都没建出来，不该发出去")
	}
}

type nilHeaders struct{ base string }

func (n nilHeaders) BuildHeaders(context.Context) map[string]string { return nil }
func (n nilHeaders) BaseURL() string                                { return n.base }

func TestBridge_构头返回nil不发请求(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = httpx.HeaderProviderFunc{Base: srv.URL, Build: func(context.Context) map[string]string { return nil }}
	})
	_, err := c.Get(context.Background(), "/x", nil)
	assertNilBuildHeaders(t, err, "httpx.HeaderProviderFunc")
	if srv.count() != 0 {
		t.Fatal("构头失败时不应发出网络请求")
	}
}

func TestBridge_自定义Provider构头nil带类型名(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Headers = nilHeaders{base: srv.URL}
	})
	_, err := c.Get(context.Background(), "/x", nil)
	assertNilBuildHeaders(t, err, "interceptor_test.nilHeaders")
	if srv.count() != 0 {
		t.Fatal("构头失败时不应发出网络请求")
	}
}

type failReadCloser struct{ err error }

func (f failReadCloser) Read([]byte) (int, error) { return 0, f.err }
func (f failReadCloser) Close() error             { return nil }

func TestBodyDecode_读体失败分流(t *testing.T) {
	t.Run("普通读错", func(t *testing.T) {
		cause := errors.New("disk read failed")
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: "https://x.example"},
			Interceptors: httpx.Interceptors{
				interceptor.NewBodyDecodeInterceptor(),
				httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
					return &httpx.Response{
						StatusCode: 200,
						Header:     http.Header{},
						Raw:        &http.Response{Body: failReadCloser{err: cause}},
					}, nil
				}),
			},
		})
		_, err := c.Get(context.Background(), "/x", nil)
		assertReadResponseBody(t, err, httpx.EncodingIdentity, cause)
		var re *kiterrors.RetryableError
		if errors.As(err, &re) {
			t.Fatal("普通读错不该变成 RetryableError")
		}
	})
	t.Run("未识别编码保留原文", func(t *testing.T) {
		cause := errors.New("disk read failed")
		header := http.Header{}
		header.Set("content-encoding", "compress") // 本库不解压 compress，Encoding 会收成 Identity
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: "https://x.example"},
			Interceptors: httpx.Interceptors{
				interceptor.NewBodyDecodeInterceptor(),
				httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
					return &httpx.Response{
						StatusCode: 200,
						Header:     header,
						Raw:        &http.Response{Body: failReadCloser{err: cause}},
					}, nil
				}),
			},
		})
		_, err := c.Get(context.Background(), "/x", nil)
		var got *httpx.ReadResponseBodyError
		if !errors.As(err, &got) {
			t.Fatalf("err = %v (%T), want *httpx.ReadResponseBodyError", err, err)
		}
		t.Logf("Encoding=%q RawEncoding=%q", got.Encoding, got.RawEncoding)
		if got.Encoding != httpx.EncodingIdentity {
			t.Fatalf("Encoding = %q, want EncodingIdentity（未登记编码归一化为 Identity）", got.Encoding)
		}
		if got.RawEncoding != "compress" {
			t.Fatalf("RawEncoding = %q, want %q（未识别编码原文必须保留供排障）", got.RawEncoding, "compress")
		}
	})
	t.Run("可重试网络读错", func(t *testing.T) {
		cause := errors.New("connection reset by peer")
		c := newClientWith(t, httpx.Options{
			Headers: httpx.StaticHeaders{Base: "https://x.example"},
			Interceptors: httpx.Interceptors{
				interceptor.NewBodyDecodeInterceptor(),
				httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
					return &httpx.Response{
						StatusCode: 200,
						Header:     http.Header{},
						Raw:        &http.Response{Body: failReadCloser{err: cause}},
					}, nil
				}),
			},
		})
		_, err := c.Get(context.Background(), "/x", nil)
		var re *kiterrors.RetryableError
		if !errors.As(err, &re) {
			t.Fatalf("err = %v (%T), want *errors.RetryableError", err, err)
		}
		var rd *httpx.ReadResponseBodyError
		if errors.As(err, &rd) {
			t.Fatal("可重试网络读错不该变成 ReadResponseBodyError")
		}
		t.Logf("RetryableError Attempts=%d err=%v", re.Attempts, err)
	})
}

func TestNoRedirectTerminal_传输错误也包成TransportError(t *testing.T) {
	// 禁重定向终端和默认终端必须对错误做同样的包装，否则那条链上的重试会失效。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	var attempts int
	c := newClientWith(t, httpx.Options{
		Headers:      httpx.StaticHeaders{Base: srv.URL},
		Retry:        httpx.WithRetry(2, time.Millisecond, 0),
		Interceptors: httpx.SpliceBeforeTerminal(interceptor.NoRedirectChain(), countingInterceptor(&attempts)),
	})
	_, err := c.Get(context.Background(), "/x", nil)
	t.Logf("noRedirect transport err=%v attempts=%d", err, attempts)
	if err == nil {
		t.Fatal("want error")
	}
	if attempts != 3 {
		t.Fatalf("尝试 %d 次，want 3(禁重定向终端的错误同样应触发重试)", attempts)
	}
}

func countingInterceptor(n *int) httpx.Interceptor {
	return httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
		*n++
		return ch.Proceed()
	})
}
