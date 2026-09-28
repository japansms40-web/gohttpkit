package interceptor_test

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
	"github.com/klauspost/compress/zstd"
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
	body, err := c.Get(t.Context(), "/x", nil)
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
		_, err := c.Get(t.Context(), "/x", nil)
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
		_, err := c.Get(t.Context(), "/x", nil)
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

type failReadCloser struct{ err error }

func (f failReadCloser) Read([]byte) (int, error) { return 0, f.err }

func (f failReadCloser) Close() error { return nil }

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
		_, err := c.Get(t.Context(), "/x", nil)
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
		_, err := c.Get(t.Context(), "/x", nil)
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
		_, err := c.Get(t.Context(), "/x", nil)
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

func TestBodyDecode_Close失败仍返回体(t *testing.T) {
	buf := captureInterceptorLogs(t)
	closeErr := errors.New("close pipe")
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewBodyDecodeInterceptor(),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) {
				return &httpx.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Raw:        &http.Response{Body: eofCloseFail{closeErr: closeErr}},
				}, nil
			}),
		},
	})
	// InterceptorFunc 不是终端……但 Proceed 会调用它并返回，不需要终端如果它是最后一层？
	// Chain.Proceed: 如果 index >= len，ChainExhausted。所以最后一层必须自己不 Proceed，或是终端。
	// InterceptorFunc 会作为最后一层被调用，只要它不 Proceed 就行。OK。
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("Close 失败 body=%q err=%v logs=%s", body, err, buf.String())
	if err != nil {
		t.Fatalf("Close 失败不应淹没已读体，err=%v", err)
	}
	if string(body) != "" {
		t.Fatalf("EOF 体应为空，got %q", body)
	}
	if !strings.Contains(buf.String(), "failed to close response body") {
		t.Fatal("Close 失败应打 logger.Error")
	}
}

type eofCloseFail struct{ closeErr error }

func (e eofCloseFail) Read([]byte) (int, error) { return 0, io.EOF }

func (e eofCloseFail) Close() error { return e.closeErr }

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
		got, err := c.Get(t.Context(), "/x", nil)
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
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("损坏 gzip → err=%v", err)
	var ce *httpx.ContentEncodingError
	if !errors.As(err, &ce) {
		t.Fatalf("err=%v，应为 *ContentEncodingError", err)
	}
	if ce.Encoding != httpx.EncodingGzip {
		t.Fatalf("Encoding=%q，应为 gzip", ce.Encoding)
	}
}
