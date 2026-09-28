package interceptor_test

import (
	stderrors "errors"
	"net/http"
	"testing"

	liberrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestAPIChain_nil归类_空体404变error(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404 空体
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.Interceptors = interceptor.APIChain(nil) })
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("APIChain(nil) 404 空体 → err=%v", err)
	var se *liberrors.HTTPStatusError
	if !stderrors.As(err, &se) {
		t.Fatalf("err=%v，应为 *HTTPStatusError（空体非 2xx）", err)
	}
	if se.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode=%d，应为 404", se.StatusCode)
	}
}

func TestAPIChain_归类命中body变error(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"banned"}`)) // 200 带体
	})
	sentinel := stderrors.New("banned")
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = interceptor.APIChain(func(status int, body []byte) error {
			if status == 200 && len(body) > 0 {
				return sentinel
			}
			return nil
		})
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("APIChain(fn) 命中 → err=%v", err)
	if !stderrors.Is(err, sentinel) {
		t.Fatalf("err=%v，应为归类返回的 sentinel", err)
	}
}
