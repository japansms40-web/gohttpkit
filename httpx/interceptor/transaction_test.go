package interceptor_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/httpx/interceptor"
)

func TestTransactionInterceptor_sink为nil与出错不回调(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(), interceptor.NewTransactionInterceptor(nil))
	})
	if _, err := c.Get(t.Context(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("sink=nil ok")

	var called int
	c2 := newClientWith(t, httpx.Options{
		Headers: httpx.StaticHeaders{Base: "https://x.example"},
		Interceptors: httpx.Interceptors{
			interceptor.NewTransactionInterceptor(func(*httpx.Transaction) { called++ }),
			httpx.InterceptorFunc(func(*httpx.Chain) (*httpx.Response, error) { return nil, errors.New("boom") }),
		},
	})
	_, err := c2.Get(t.Context(), "/x", nil)
	t.Logf("resp=nil err=%v called=%d", err, called)
	if err == nil {
		t.Fatal("want error")
	}
	if called != 0 {
		t.Fatal("resp 为 nil 时不该回调")
	}
}

func TestTransaction_成功sink字段与Clone隔离(t *testing.T) {
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-server", "kit")
		w.Header().Add("set-cookie", "a=1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})
	var txn *httpx.Transaction
	var liveHeader http.Header
	c := newClient(t, srv.Server, func(o *httpx.Options) {
		o.Transport = &http.Transport{}
		o.ProxyURL = "socks5://gw.example:1080"
		o.ExitIP = "203.0.113.10"
		o.ASN = "AS64500"
		o.Interceptors = httpx.Prepend(interceptor.DefaultChain(),
			interceptor.NewTransactionInterceptor(func(tx *httpx.Transaction) { txn = tx }),
			httpx.InterceptorFunc(func(ch *httpx.Chain) (*httpx.Response, error) {
				resp, err := ch.Proceed()
				if resp != nil {
					liveHeader = resp.Header
				}
				return resp, err
			}))
	})
	if _, err := c.PostForm(t.Context(), "/create", "a=1"); err != nil {
		t.Fatal(err)
	}
	if liveHeader != nil {
		liveHeader.Set("x-server", "mutated-after-sink")
	}
	if txn == nil {
		t.Fatal("成功路径 sink 必须回调")
	}
	t.Logf("txn method=%s status=%d req=%q resp=%q proxy=%q exit=%q asn=%q dur=%d x-server=%v",
		txn.Method, txn.Status, txn.ReqBody, txn.RespBody, txn.Proxy, txn.ExitIP, txn.ASN, txn.DurationMS, txn.RespHeaders["X-Server"])
	if txn.Method != http.MethodPost || txn.Status != http.StatusCreated {
		t.Fatalf("method/status = %s/%d", txn.Method, txn.Status)
	}
	if txn.ReqBody != "a=1" || txn.ReqBodyLen != 3 {
		t.Fatalf("ReqBody=%q len=%d", txn.ReqBody, txn.ReqBodyLen)
	}
	if txn.RespBody != "created" || txn.RespBodyLen != 7 {
		t.Fatalf("RespBody=%q len=%d", txn.RespBody, txn.RespBodyLen)
	}
	if txn.Proxy != "socks5://gw.example:1080" || txn.ExitIP != "203.0.113.10" || txn.ASN != "AS64500" {
		t.Fatalf("出口元数据丢失: %+v", txn)
	}
	if txn.DurationMS < 0 {
		t.Fatalf("DurationMS=%d", txn.DurationMS)
	}
	gotServer := ""
	for k, vs := range txn.RespHeaders {
		if strings.EqualFold(k, "x-server") && len(vs) > 0 {
			gotServer = vs[0]
		}
	}
	if gotServer != "kit" {
		t.Fatalf("Clone 隔离失败：后续改 Header 污染了快照, got %q headers=%v", gotServer, txn.RespHeaders)
	}
}
