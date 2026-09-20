package netproxy

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func assertUnsupportedScheme(t *testing.T, err error, scheme string, rawDialer bool) {
	t.Helper()
	var got *UnsupportedProxySchemeError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *UnsupportedProxySchemeError", err, err)
	}
	if got.Scheme != scheme {
		t.Fatalf("Scheme = %q, want %q", got.Scheme, scheme)
	}
	if got.RawDialer != rawDialer {
		t.Fatalf("RawDialer = %v, want %v", got.RawDialer, rawDialer)
	}
	t.Logf("errors.As → *UnsupportedProxySchemeError Scheme=%q RawDialer=%v err=%v", got.Scheme, got.RawDialer, err)
}

func assertInvalidProxyURL(t *testing.T, err error, reason InvalidURLReason) {
	t.Helper()
	var got *InvalidProxyURLError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *InvalidProxyURLError", err, err)
	}
	if got.Reason != reason {
		t.Fatalf("Reason = %q, want %q", got.Reason, reason)
	}
	t.Logf("errors.As → *InvalidProxyURLError Reason=%q err=%v", got.Reason, err)
}

func TestUnsupportedProxySchemeError_Transport文案(t *testing.T) {
	err := &UnsupportedProxySchemeError{Scheme: "ftp", RawDialer: false}
	assertUnsupportedScheme(t, err, "ftp", false)
	want := "proxy: unsupported scheme: ftp (supported: socks5, http, https)"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestUnsupportedProxySchemeError_裸Dialer文案(t *testing.T) {
	err := &UnsupportedProxySchemeError{Scheme: "http", RawDialer: true}
	assertUnsupportedScheme(t, err, "http", true)
	want := "proxy: unsupported scheme for raw dialer: http, only socks5 is supported (use ApplyProxyToTransport for http/https)"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestUnsupportedProxySchemeError_nil接收者(t *testing.T) {
	got := (*UnsupportedProxySchemeError)(nil).Error()
	t.Logf("(*UnsupportedProxySchemeError)(nil).Error() = %q", got)
	if got != "netproxy: unsupported proxy scheme <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestUnsupportedProxySchemeError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("configure transport: %w", &UnsupportedProxySchemeError{Scheme: "ftp", RawDialer: false})
	t.Logf("wrapped = %v", wrapped)
	assertUnsupportedScheme(t, wrapped, "ftp", false)
}

func TestInvalidProxyURLError_parse文案(t *testing.T) {
	err := &InvalidProxyURLError{Reason: InvalidURLReasonParse}
	assertInvalidProxyURL(t, err, InvalidURLReasonParse)
	want := "proxy: parse url"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestInvalidProxyURLError_缺host或port文案(t *testing.T) {
	err := &InvalidProxyURLError{Reason: InvalidURLReasonMissingHostOrPort}
	assertInvalidProxyURL(t, err, InvalidURLReasonMissingHostOrPort)
	want := "proxy: missing host or port"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestInvalidProxyURLError_nil接收者(t *testing.T) {
	got := (*InvalidProxyURLError)(nil).Error()
	t.Logf("(*InvalidProxyURLError)(nil).Error() = %q", got)
	if got != "netproxy: invalid proxy url <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestInvalidProxyURLError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("configure dialer: %w", &InvalidProxyURLError{Reason: InvalidURLReasonParse})
	t.Logf("wrapped = %v", wrapped)
	assertInvalidProxyURL(t, wrapped, InvalidURLReasonParse)
}

func TestInvalidProxyURLError_只有Reason字段(t *testing.T) {
	typ := reflect.TypeOf(InvalidProxyURLError{})
	if typ.NumField() != 1 {
		t.Fatalf("InvalidProxyURLError 字段数 = %d, want 1（不得存 Raw/Err）", typ.NumField())
	}
	f := typ.Field(0)
	t.Logf("唯一字段 %s %s", f.Name, f.Type)
	if f.Name != "Reason" || f.Type != reflect.TypeOf(InvalidURLReason("")) {
		t.Fatalf("唯一字段应为 Reason InvalidURLReason，得到 %s %s", f.Name, f.Type)
	}
}

func TestNetproxy错误类型互不误匹配(t *testing.T) {
	scheme := &UnsupportedProxySchemeError{Scheme: "ftp"}
	invalid := &InvalidProxyURLError{Reason: InvalidURLReasonParse}

	var asScheme *UnsupportedProxySchemeError
	var asInvalid *InvalidProxyURLError
	if errors.As(invalid, &asScheme) {
		t.Fatal("InvalidProxyURLError 不应被 As 成 *UnsupportedProxySchemeError")
	}
	if errors.As(scheme, &asInvalid) {
		t.Fatal("UnsupportedProxySchemeError 不应被 As 成 *InvalidProxyURLError")
	}
	t.Logf("两类错误互不误匹配")
}

func TestUnsupportedDialerError_文案与nil接收者(t *testing.T) {
	err := &UnsupportedDialerError{}
	t.Logf("Error() = %q", err.Error())
	if err.Error() != "proxy: dialer must implement proxy.ContextDialer" {
		t.Errorf("Error() = %q", err.Error())
	}
	got := (*UnsupportedDialerError)(nil).Error()
	t.Logf("(*UnsupportedDialerError)(nil).Error() = %q", got)
	if got != "netproxy: unsupported dialer <nil>" {
		t.Errorf("nil 接收者 Error() = %q", got)
	}
}

func TestUnsupportedDialerError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("build dial context: %w", &UnsupportedDialerError{})
	t.Logf("wrapped = %v", wrapped)
	var got *UnsupportedDialerError
	if !errors.As(wrapped, &got) {
		t.Fatalf("err = %v (%T), want *UnsupportedDialerError", wrapped, wrapped)
	}
}

func TestApplyProxyToTransport_ftp是UnsupportedScheme(t *testing.T) {
	err := ApplyProxyToTransport(&http.Transport{}, "ftp://h:1")
	t.Logf("ftp → err=%v", err)
	assertUnsupportedScheme(t, err, "ftp", false)
}

func TestParseProxyURL_http是裸DialerUnsupportedScheme(t *testing.T) {
	d, err := ParseProxyURL("http://127.0.0.1:8080")
	t.Logf("http → dialer=%v err=%v", d, err)
	if d != nil {
		t.Fatal("失败时 dialer 必须为 nil")
	}
	assertUnsupportedScheme(t, err, "http", true)
}

func TestParseAndValidate_缺端口是InvalidProxyURL(t *testing.T) {
	err := ApplyProxyToTransport(&http.Transport{}, "socks5://127.0.0.1")
	t.Logf("缺端口 → err=%v", err)
	assertInvalidProxyURL(t, err, InvalidURLReasonMissingHostOrPort)
}

func Test空代理URL不是错误(t *testing.T) {
	d, err := ParseProxyURL("")
	t.Logf(`ParseProxyURL("") → dialer=%v err=%v`, d, err)
	if d != nil || err != nil {
		t.Fatal(`ParseProxyURL("") 应是 (nil, nil)`)
	}
	if err := ApplyProxyToTransport(nil, ""); err != nil {
		t.Fatalf(`ApplyProxyToTransport(nil, "") = %v, want nil`, err)
	}
}

func Test非法代理URL文案不泄漏凭据(t *testing.T) {
	raw := "socks5://user:secret@%zz"
	err := ApplyProxyToTransport(&http.Transport{}, raw)
	t.Logf("leaky-url → err=%v (%T)", err, err)
	assertInvalidProxyURL(t, err, InvalidURLReasonParse)
	msg := err.Error()
	for _, secret := range []string{"user", "secret", raw, "user:secret"} {
		if strings.Contains(msg, secret) {
			t.Errorf("Error() = %q 含敏感串 %q", msg, secret)
		}
	}
	// %+v 打结构体也不得带出 raw / 底层 parse err。
	dumped := fmt.Sprintf("%+v", err)
	t.Logf("%+v = %s", err, dumped)
	for _, secret := range []string{"user", "secret", raw} {
		if strings.Contains(dumped, secret) {
			t.Errorf("%%+v = %q 含敏感串 %q", dumped, secret)
		}
	}
}
