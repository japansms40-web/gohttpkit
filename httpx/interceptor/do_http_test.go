package interceptor_test

// do_http_test.go —— doHTTP 是两种终端（callServer / noRedirect）的共用路径，这里对两者跑同一组契约：
// 首次尝试快照请求头、响应字段取自 *http.Response 且 Raw.Body 未读、网络失败包成 *httpx.TransportError。

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

var terminals = []struct {
	name string
	new  func() httpx.Interceptor
}{
	{"callServer", interceptor.NewCallServerInterceptor},
	{"noRedirect", interceptor.NewNoRedirectCallServerInterceptor},
}

func TestDoHTTP_快照请求头与响应字段且体未读(t *testing.T) {
	for _, tm := range terminals {
		t.Run(tm.name, func(t *testing.T) {
			srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-srv", "v")
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte("raw-body"))
			})
			var reqHeaders http.Header
			var status, protoMajor int
			var proto, srvHeader, rawBody string
			probe := httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
				resp, err := ch.Proceed()
				if err != nil {
					return nil, err
				}
				reqHeaders = ch.Request().ReqHeaders
				status, proto, protoMajor, srvHeader = resp.StatusCode, resp.Proto, resp.ProtoMajor, resp.Header.Get("x-srv")
				b, _ := io.ReadAll(resp.Raw.Body)
				_ = resp.Raw.Body.Close()
				rawBody = string(b)
				return resp, nil
			})
			c := newClient(t, srv.Server, func(o *httpx.Options) {
				o.Interceptors = httpx.Interceptors{probe, interceptor.NewBridgeInterceptor(), tm.new()}
			})
			_, err := c.Get(t.Context(), "/x", nil)
			t.Logf("err=%v status=%d proto=%q/%d x-srv=%q raw=%q reqHeaders=%v",
				err, status, proto, protoMajor, srvHeader, rawBody, reqHeaders)
			if err != nil {
				t.Fatal(err)
			}
			// 快照保留白名单里的原样键名（小写），不能用规范化的 Header.Get 取，按普通 map 读。
			raw := map[string][]string(reqHeaders)
			if got := raw["accept"]; len(got) != 1 || got[0] != "application/json" {
				t.Fatalf("应快照实际发出的 accept 头，得到 %v", reqHeaders)
			}
			if got := raw[httpx.HeaderHost]; len(got) != 1 || got[0] != srv.Listener.Addr().String() {
				t.Fatalf("应快照实际发出的请求头与 Host，得到 %v", reqHeaders)
			}
			if status != http.StatusTeapot || proto != "HTTP/1.1" || protoMajor != 1 || srvHeader != "v" {
				t.Fatalf("响应字段应取自 *http.Response：status=%d proto=%q/%d x-srv=%q", status, proto, protoMajor, srvHeader)
			}
			if rawBody != "raw-body" {
				t.Fatalf("终端不应读体（所有权移交 bodyDecode），Raw.Body 读到 %q", rawBody)
			}
		})
	}
}

func TestDoHTTP_网络失败包成TransportError并保留底层错误(t *testing.T) {
	for _, tm := range terminals {
		t.Run(tm.name, func(t *testing.T) {
			dead := httptest.NewServer(http.NotFoundHandler())
			base := dead.URL
			dead.Close() // 端口已关：连接被拒
			c := newClientWith(t, httpx.Options{
				Headers:      httpx.StaticHeaders{Base: base},
				Interceptors: httpx.Interceptors{interceptor.NewBridgeInterceptor(), tm.new()},
			})
			_, err := c.Get(t.Context(), "/x", nil)
			t.Logf("err=%v (%T)", err, err)
			var te *httpx.TransportError
			if !errors.As(err, &te) {
				t.Fatalf("网络失败应是 *httpx.TransportError（否则重试层看不见），得到 %T %v", err, err)
			}
			var opErr *net.OpError
			if !errors.As(te, &opErr) || opErr.Op != "dial" {
				t.Fatalf("TransportError 应保留底层 dial 错误，得到 %v", te.Err)
			}
		})
	}
}
