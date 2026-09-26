package interceptor_test

// boundary_more_test.go —— 补角度测试：把之前只有单用例/未测的分支（APIChain 装配、
// HTML 提纯与错误页标记、非 2xx 状态语义、归类报错、特殊头消化）逐个焊死。

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

func TestStatusSemantics_非2xx带体放行(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"msg":"bad"}`)) // 400 带体：DefaultStatusRule 放行
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewStatusSemanticsInterceptor(nil))
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("400 带体 → body=%q err=%v", body, err)
	if err != nil {
		t.Fatalf("非 2xx 带体应放行（4xx 常承载业务体），却 err=%v", err)
	}
	if string(body) != `{"msg":"bad"}` {
		t.Fatalf("body=%q", body)
	}
}

func TestHTMLText_提纯剥script(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><script>var t='secret'</script><p>你好 世界</p></body></html>"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor())
	})
	body, err := c.Get(t.Context(), "/x", nil)
	t.Logf("HTML 提纯 → body=%q err=%v", body, err)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if got != "你好 世界" {
		t.Fatalf("提纯结果=%q，应剥掉 script、只留可见文案", got)
	}
}

func TestHTMLText_命中错误页标记报错(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte("<html><body>Access Denied</body></html>"))
	})
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewHTMLTextInterceptor("", "Access Denied"))
	})
	_, err := c.Get(t.Context(), "/x", nil)
	t.Logf("HTML 命中标记 → err=%v", err)
	var se *liberrors.HTTPStatusError
	if !stderrors.As(err, &se) {
		t.Fatalf("err=%v，应为命中标记的 *HTTPStatusError", err)
	}
}

func TestApplySpecialHeaders_host头消化(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) { o.Interceptors = interceptor.DefaultChain() })
	_, err := c.Do(t.Context(), httpx.RequestSpec{
		Path:         "/x",
		ExtraHeaders: map[string]string{"host": "custom.example.test", "content-length": "not-a-number"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 取服务器实际收到的请求，Host 应被 applySpecialHeaders 写进 Request.Host。
	<-srv.mu
	last := srv.requests[len(srv.requests)-1]
	srv.mu <- struct{}{}
	t.Logf("服务器收到 Host=%q（非法 content-length 已被静默丢弃、不崩）", last.Host)
	if last.Host != "custom.example.test" {
		t.Fatalf("Host=%q，应被 host 头覆盖为 custom.example.test", last.Host)
	}
}
