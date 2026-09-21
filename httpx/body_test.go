package httpx_test

// body_test.go —— 请求体编码、响应 JSON 解码、日志截断的单元契约。

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

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
