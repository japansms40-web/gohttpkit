package httpx_test

// errors_test.go —— 本库类型错误的字段、nil、解包与互不误匹配。

import (
	"errors"
	"fmt"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

func assertMissingHeaderProvider(t *testing.T, err error, field string) {
	t.Helper()
	var got *httpx.MissingHeaderProviderError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.MissingHeaderProviderError", err, err)
	}
	if got.Field != field {
		t.Fatalf("Field = %q, want %q", got.Field, field)
	}
	t.Logf("errors.As → *MissingHeaderProviderError Field=%q err=%v", got.Field, err)
}

func assertChainExhausted(t *testing.T, err error, index, length int) {
	t.Helper()
	var got *httpx.ChainExhaustedError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.ChainExhaustedError", err, err)
	}
	if got.Index != index || got.Length != length {
		t.Fatalf("Index/Length = %d/%d, want %d/%d", got.Index, got.Length, index, length)
	}
	t.Logf("errors.As → *ChainExhaustedError Index=%d Length=%d err=%v", got.Index, got.Length, err)
}

func assertNilBuildHeaders(t *testing.T, err error, providerType string) {
	t.Helper()
	var got *httpx.NilBuildHeadersError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.NilBuildHeadersError", err, err)
	}
	if got.ProviderType != providerType {
		t.Fatalf("ProviderType = %q, want %q", got.ProviderType, providerType)
	}
	t.Logf("errors.As → *NilBuildHeadersError ProviderType=%q err=%v", got.ProviderType, err)
}

func assertRequestBodyEncode(t *testing.T, err error, bodyType string, cause error) {
	t.Helper()
	var got *httpx.RequestBodyEncodeError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.RequestBodyEncodeError", err, err)
	}
	if got.BodyType != bodyType || !errors.Is(got, cause) {
		t.Fatalf("BodyType=%q cause=%v, want %q / %v", got.BodyType, got.Err, bodyType, cause)
	}
	t.Logf("errors.As → *RequestBodyEncodeError BodyType=%q err=%v", got.BodyType, err)
}

func assertResponseJSONDecode(t *testing.T, err error, targetType string, cause error) {
	t.Helper()
	var got *httpx.ResponseJSONDecodeError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.ResponseJSONDecodeError", err, err)
	}
	if got.TargetType != targetType || !errors.Is(got, cause) {
		t.Fatalf("TargetType=%q cause=%v, want %q / %v", got.TargetType, got.Err, targetType, cause)
	}
	t.Logf("errors.As → *ResponseJSONDecodeError TargetType=%q err=%v", got.TargetType, err)
}

func assertCreateHTTPRequest(t *testing.T, err error, method string, cause error) {
	t.Helper()
	var got *httpx.CreateHTTPRequestError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.CreateHTTPRequestError", err, err)
	}
	if got.Method != method || !errors.Is(got, cause) {
		t.Fatalf("Method=%q cause=%v, want %q / %v", got.Method, got.Err, method, cause)
	}
	t.Logf("errors.As → *CreateHTTPRequestError Method=%q err=%v", got.Method, err)
}

func assertContentEncoding(t *testing.T, err error, encoding string, cause error) {
	t.Helper()
	var got *httpx.ContentEncodingError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.ContentEncodingError", err, err)
	}
	if got.Encoding != encoding || !errors.Is(got, cause) {
		t.Fatalf("Encoding=%q cause=%v, want %q / %v", got.Encoding, got.Err, encoding, cause)
	}
	t.Logf("errors.As → *ContentEncodingError Encoding=%q err=%v", got.Encoding, err)
}

func assertReadResponseBody(t *testing.T, err error, encoding string, cause error) {
	t.Helper()
	var got *httpx.ReadResponseBodyError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.ReadResponseBodyError", err, err)
	}
	if got.Encoding != encoding || !errors.Is(got, cause) {
		t.Fatalf("Encoding=%q cause=%v, want %q / %v", got.Encoding, got.Err, encoding, cause)
	}
	t.Logf("errors.As → *ReadResponseBodyError Encoding=%q err=%v", got.Encoding, err)
}

func TestMissingHeaderProviderError_字段与文案(t *testing.T) {
	err := &httpx.MissingHeaderProviderError{Field: "Options.Headers"}
	assertMissingHeaderProvider(t, err, "Options.Headers")
	want := "httpx: Options.Headers is required (implement HeaderProvider; StaticHeaders is the simplest)"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestMissingHeaderProviderError_nil接收者(t *testing.T) {
	got := (*httpx.MissingHeaderProviderError)(nil).Error()
	t.Logf("nil.Error() = %q", got)
	if got != "httpx: missing header provider <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestMissingHeaderProviderError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("new client: %w", &httpx.MissingHeaderProviderError{Field: "Options.Headers"})
	assertMissingHeaderProvider(t, wrapped, "Options.Headers")
}

func TestChainExhaustedError_字段与文案(t *testing.T) {
	err := &httpx.ChainExhaustedError{Index: 1, Length: 1}
	assertChainExhausted(t, err, 1, 1)
	want := "httpx: interceptor chain exhausted — missing terminal interceptor (e.g. NewCallServerInterceptor)"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestChainExhaustedError_nil接收者(t *testing.T) {
	got := (*httpx.ChainExhaustedError)(nil).Error()
	t.Logf("nil.Error() = %q", got)
	if got != "httpx: interceptor chain exhausted <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestChainExhaustedError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("proceed: %w", &httpx.ChainExhaustedError{Index: 0, Length: 0})
	assertChainExhausted(t, wrapped, 0, 0)
}

func TestNilBuildHeadersError_字段与文案(t *testing.T) {
	err := &httpx.NilBuildHeadersError{ProviderType: "httpx.HeaderProviderFunc"}
	assertNilBuildHeaders(t, err, "httpx.HeaderProviderFunc")
	assertNilBuildHeaders(t, &httpx.NilBuildHeadersError{ProviderType: "httpx_test.nilHeaders"}, "httpx_test.nilHeaders")
	want := "httpx: HeaderProvider.BuildHeaders returned nil"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestNilBuildHeadersError_nil接收者(t *testing.T) {
	got := (*httpx.NilBuildHeadersError)(nil).Error()
	t.Logf("nil.Error() = %q", got)
	if got != "httpx: nil build headers <nil>" {
		t.Errorf("Error() = %q", got)
	}
}

func TestNilBuildHeadersError_包装后仍可As(t *testing.T) {
	wrapped := fmt.Errorf("bridge: %w", &httpx.NilBuildHeadersError{ProviderType: "httpx.HeaderProviderFunc"})
	assertNilBuildHeaders(t, wrapped, "httpx.HeaderProviderFunc")
}

func TestRequestBodyEncodeError_字段解包与文案(t *testing.T) {
	cause := errors.New("json: unsupported type: chan int")
	err := &httpx.RequestBodyEncodeError{BodyType: "chan int", Err: cause}
	assertRequestBodyEncode(t, err, "chan int", cause)
	want := "httpx: encode request body as JSON (chan int): json: unsupported type: chan int"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestRequestBodyEncodeError_nil接收者与空Err(t *testing.T) {
	if got := (*httpx.RequestBodyEncodeError)(nil).Error(); got != "httpx: encode request body <nil>" {
		t.Errorf("nil.Error() = %q", got)
	}
	if got := (*httpx.RequestBodyEncodeError)(nil).Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v", got)
	}
	empty := &httpx.RequestBodyEncodeError{BodyType: "chan int"}
	t.Logf("Err==nil Error() = %q Unwrap=%v", empty.Error(), empty.Unwrap())
	if empty.Unwrap() != nil {
		t.Fatal("Err==nil 时 Unwrap 应为 nil")
	}
}

func TestRequestBodyEncodeError_包装后仍可As(t *testing.T) {
	cause := errors.New("json boom")
	wrapped := fmt.Errorf("do: %w", &httpx.RequestBodyEncodeError{BodyType: "chan int", Err: cause})
	assertRequestBodyEncode(t, wrapped, "chan int", cause)
}

func TestResponseJSONDecodeError_字段解包与文案(t *testing.T) {
	cause := errors.New("invalid character")
	err := &httpx.ResponseJSONDecodeError{TargetType: "*struct { OK bool }", Err: cause}
	assertResponseJSONDecode(t, err, "*struct { OK bool }", cause)
	want := "httpx: decode response JSON: invalid character"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestResponseJSONDecodeError_nil接收者与空Err(t *testing.T) {
	if got := (*httpx.ResponseJSONDecodeError)(nil).Error(); got != "httpx: decode response JSON <nil>" {
		t.Errorf("nil.Error() = %q", got)
	}
	if got := (*httpx.ResponseJSONDecodeError)(nil).Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v", got)
	}
	if got := (&httpx.ResponseJSONDecodeError{}).Unwrap(); got != nil {
		t.Errorf("empty.Unwrap() = %v", got)
	}
}

func TestResponseJSONDecodeError_包装后仍可As(t *testing.T) {
	cause := errors.New("syntax")
	wrapped := fmt.Errorf("decode: %w", &httpx.ResponseJSONDecodeError{TargetType: "*int", Err: cause})
	assertResponseJSONDecode(t, wrapped, "*int", cause)
}

func TestCreateHTTPRequestError_字段解包与文案(t *testing.T) {
	cause := errors.New("net/http: invalid method")
	err := &httpx.CreateHTTPRequestError{Method: "BAD METHOD", Err: cause}
	assertCreateHTTPRequest(t, err, "BAD METHOD", cause)
	want := "failed to create request: net/http: invalid method"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestCreateHTTPRequestError_nil接收者与空Err(t *testing.T) {
	if got := (*httpx.CreateHTTPRequestError)(nil).Error(); got != "httpx: create http request <nil>" {
		t.Errorf("nil.Error() = %q", got)
	}
	if got := (*httpx.CreateHTTPRequestError)(nil).Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v", got)
	}
}

func TestCreateHTTPRequestError_包装后仍可As(t *testing.T) {
	cause := errors.New("invalid method")
	wrapped := fmt.Errorf("bridge: %w", &httpx.CreateHTTPRequestError{Method: "GET", Err: cause})
	assertCreateHTTPRequest(t, wrapped, "GET", cause)
}

func TestContentEncodingError_字段解包与文案(t *testing.T) {
	cause := errors.New("gzip: invalid header")
	err := &httpx.ContentEncodingError{Encoding: "gzip", Err: cause}
	assertContentEncoding(t, err, "gzip", cause)
	want := "failed to create gzip reader: gzip: invalid header"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestContentEncodingError_nil接收者与空Err(t *testing.T) {
	if got := (*httpx.ContentEncodingError)(nil).Error(); got != "httpx: content encoding <nil>" {
		t.Errorf("nil.Error() = %q", got)
	}
	if got := (*httpx.ContentEncodingError)(nil).Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v", got)
	}
}

func TestContentEncodingError_包装后仍可As(t *testing.T) {
	cause := errors.New("zstd: bad")
	wrapped := fmt.Errorf("decode: %w", &httpx.ContentEncodingError{Encoding: "zstd", Err: cause})
	assertContentEncoding(t, wrapped, "zstd", cause)
}

func TestReadResponseBodyError_字段解包与文案(t *testing.T) {
	cause := errors.New("disk read failed")
	err := &httpx.ReadResponseBodyError{Encoding: "gzip", Err: cause}
	assertReadResponseBody(t, err, "gzip", cause)
	want := "failed to read response: disk read failed"
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestReadResponseBodyError_nil接收者与空Err(t *testing.T) {
	if got := (*httpx.ReadResponseBodyError)(nil).Error(); got != "httpx: read response body <nil>" {
		t.Errorf("nil.Error() = %q", got)
	}
	if got := (*httpx.ReadResponseBodyError)(nil).Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v", got)
	}
}

func TestReadResponseBodyError_包装后仍可As(t *testing.T) {
	cause := errors.New("short read")
	wrapped := fmt.Errorf("body: %w", &httpx.ReadResponseBodyError{Encoding: "", Err: cause})
	assertReadResponseBody(t, wrapped, "", cause)
}

func TestHttpx错误类型互不误匹配(t *testing.T) {
	all := []error{
		&httpx.MissingHeaderProviderError{Field: "Options.Headers"},
		&httpx.ChainExhaustedError{Index: 1, Length: 1},
		&httpx.NilBuildHeadersError{ProviderType: "x"},
		&httpx.RequestBodyEncodeError{BodyType: "chan int", Err: errors.New("e")},
		&httpx.ResponseJSONDecodeError{TargetType: "*int", Err: errors.New("e")},
		&httpx.CreateHTTPRequestError{Method: "GET", Err: errors.New("e")},
		&httpx.ContentEncodingError{Encoding: "gzip", Err: errors.New("e")},
		&httpx.ReadResponseBodyError{Encoding: "gzip", Err: errors.New("e")},
	}
	newTargets := func() []any {
		return []any{
			new(*httpx.MissingHeaderProviderError),
			new(*httpx.ChainExhaustedError),
			new(*httpx.NilBuildHeadersError),
			new(*httpx.RequestBodyEncodeError),
			new(*httpx.ResponseJSONDecodeError),
			new(*httpx.CreateHTTPRequestError),
			new(*httpx.ContentEncodingError),
			new(*httpx.ReadResponseBodyError),
		}
	}
	t.Logf("types=%d", len(all))
	for i, err := range all {
		targets := newTargets()
		for j, target := range targets {
			hit := errors.As(err, target)
			if i == j {
				if !hit {
					t.Fatalf("%T 应匹配自身", err)
				}
				continue
			}
			if hit {
				t.Fatalf("%T 误匹配成 targets[%d]", err, j)
			}
		}
	}
}
