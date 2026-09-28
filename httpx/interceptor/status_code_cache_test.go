package interceptor_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestStatusCodeCache与响应头缓存_内层出错时穿透(t *testing.T) {
	sentinel := errors.New("inner")
	for name, chain := range map[string]httpx.Interceptors{
		"statusCodeCache":     {interceptor.NewStatusCodeCacheInterceptor(), failing(sentinel)},
		"responseHeaderCache": {interceptor.NewResponseHeaderCacheInterceptor(), failing(sentinel)},
		"bodyDecode":          {interceptor.NewBodyDecodeInterceptor(), failing(sentinel)},
	} {
		t.Run(name, func(t *testing.T) {
			c := newClientWith(t, httpx.Options{Headers: httpx.StaticHeaders{Base: "https://x.example"}, Interceptors: chain})
			_, err := c.Get(t.Context(), "/x", nil)
			t.Logf("%s err=%v is=%v", name, err, errors.Is(err, sentinel))
			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func failing(err error) httpx.Interceptor {
	return httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, err })
}

func TestStatusCodeCache_成功写入快照(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	})
	c := newClient(t, srv.Server, nil)
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("status snapshot=%d", c.SnapshotResponseStatusCode())
	if c.SnapshotResponseStatusCode() != http.StatusTeapot {
		t.Fatalf("Snapshot=%d, want 418", c.SnapshotResponseStatusCode())
	}
}
