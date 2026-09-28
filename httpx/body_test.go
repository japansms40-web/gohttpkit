package httpx_test

// body_test.go —— 请求体编码、响应 JSON 解码、日志截断的单元契约。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"unsafe"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// ─────────────────────────────── body.go ───────────────────────────────

func TestEncodeRequestBody_struct走JSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	got, err := httpx.EncodeRequestBody(payload{Name: "x", N: 1})
	t.Logf("struct → %q err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"x","n":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeRequestBody_不可序列化类型报错(t *testing.T) {
	_, err := httpx.EncodeRequestBody(make(chan int))
	t.Logf("chan int → err=%v (%T)", err, err)
	var ue *json.UnsupportedTypeError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v (%T), want 底层 json.UnsupportedTypeError", err, err)
	}
	assertRequestBodyEncode(t, err, "chan int", ue)
}

func TestDecodeResponse(t *testing.T) {
	var out struct {
		OK bool `json:"ok"`
	}
	err := httpx.DecodeResponse([]byte(`{"ok":true}`), &out)
	t.Logf("ok=true → out=%+v err=%v", out, err)
	if err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatal("未解析出字段")
	}
	err = httpx.DecodeResponse([]byte(`not json`), &out)
	t.Logf("not json → err=%v (%T)", err, err)
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v (%T), want 底层 json.SyntaxError", err, err)
	}
	assertResponseJSONDecode(t, err, fmt.Sprintf("%T", &out), se)
}

func TestTruncateBodyForLog_边界(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"短于上限原样", "abc", 10, "abc"},
		{"等于上限原样", "abc", 3, "abc"},
		{"超出则截断加占位", "abcdef", 3, "abc...(truncated, total=6)"},
		{"limit 为 0 不截断", "abcdef", 0, "abcdef"},
		{"limit 为负不截断", "abcdef", -1, "abcdef"},
		{"空体", "", 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(httpx.TruncateBodyForLog([]byte(tc.in), tc.limit))
			t.Logf("in=%q limit=%d → %q", tc.in, tc.limit, got)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEncodeRequestBody_nil与字节同引用与string与表单(t *testing.T) {
	got, err := httpx.EncodeRequestBody(nil)
	t.Logf("nil → %v err=%v", got, err)
	if got != nil || err != nil {
		t.Fatalf("nil → (%v, %v)", got, err)
	}

	in := []byte("hi")
	got, err = httpx.EncodeRequestBody(in)
	if err != nil {
		t.Fatal(err)
	}
	if unsafe.SliceData(got) != unsafe.SliceData(in) {
		t.Fatal("[]byte 应返回同一份切片，不复制")
	}

	got, err = httpx.EncodeRequestBody("a=1&b=2")
	t.Logf("string → %q", got)
	if err != nil || string(got) != "a=1&b=2" {
		t.Fatalf("string 应原样，got %q err=%v", got, err)
	}

	got, err = httpx.EncodeRequestBody(url.Values{"b": {"2"}, "a": {"1"}})
	t.Logf("url.Values → %q", got)
	if err != nil || string(got) != "a=1&b=2" {
		t.Fatalf("url.Values 应按字典序，got %q", got)
	}
}

func TestDecodeResponse_空与非指针(t *testing.T) {
	var m map[string]any
	err := httpx.DecodeResponse(nil, &m)
	t.Logf("nil data → err=%v", err)
	var de *httpx.ResponseJSONDecodeError
	if !errors.As(err, &de) {
		t.Fatalf("空输入要 *ResponseJSONDecodeError，got %v", err)
	}

	var val map[string]any
	err = httpx.DecodeResponse([]byte(`{"a":1}`), val)
	t.Logf("非指针 → err=%v TargetType=%s", err, "")
	if !errors.As(err, &de) {
		t.Fatalf("非指针要 *ResponseJSONDecodeError，got %v", err)
	}
}

func TestTruncateBodyForLog_nil体(t *testing.T) {
	got := httpx.TruncateBodyForLog(nil, 3)
	t.Logf("nil → %v nil=%v", got, got == nil)
	if got != nil {
		t.Fatalf("nil 体应原样返回 nil，got %v", got)
	}
}
