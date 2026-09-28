package interceptor_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestClassifyInterceptor_classify为nil时放行(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewClassifyInterceptor(nil))
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("classify=nil body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassifyInterceptor_内层出错时不覆盖结论(t *testing.T) {
	sentinel := errors.New("inner")
	var classified int
	c := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewClassifyInterceptor(func(int, []byte) error { classified++; return errors.New("outer") }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, sentinel }),
		},
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("inner err=%v classified=%d", err, classified)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want 内层错误原样穿透", err)
	}
	if classified != 0 {
		t.Fatal("内层已判失败时不该再跑归类")
	}
}
