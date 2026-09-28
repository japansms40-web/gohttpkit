package httpx_test

import (
	"errors"
	"testing"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
)

func TestRetryableTextRule_命中时包装HTTPStatusError(t *testing.T) {
	rule := httpx.RetryableTextRule([]string{"slow down"}, 572)
	err := rule(429, []byte("please slow down now"))
	t.Logf("429+keyword → %v (%T)", err, err)
	var re *kiterrors.RetryableError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v, want RetryableError", err)
	}
	var se *kiterrors.HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 429 {
		t.Fatalf("应能 As 到 HTTPStatusError(429), got %v", err)
	}
}

func TestRetryableTextRule_未命中时回落默认规则(t *testing.T) {
	rule := httpx.RetryableTextRule([]string{"slow down"}, 572)
	t.Logf("500+body=%v 500+empty=%v 572=%v 400+kw=%v 400+body=%v",
		rule(500, []byte("something else")), rule(500, nil), rule(572, []byte("x")),
		rule(400, []byte("please slow down now")), rule(400, []byte("normal body")))
	if err := rule(500, []byte("something else")); err != nil {
		t.Fatalf("未命中关键词且有响应体时应放行, got %v", err)
	}
	if err := rule(500, nil); err == nil {
		t.Fatal("空体应报 HTTPStatusError")
	}
	if err := rule(572, []byte("x")); err == nil {
		t.Fatal("命中状态码应报 RetryableError")
	}
	if err := rule(400, []byte("please slow down now")); err == nil {
		t.Fatal("命中关键词应报 RetryableError")
	}
	if err := rule(400, []byte("normal body")); err != nil {
		t.Fatalf("未命中且有体应放行, got %v", err)
	}
}

func TestRetryableTextRule_空关键词被忽略(t *testing.T) {
	rule := httpx.RetryableTextRule([]string{""})
	err := rule(400, []byte("anything"))
	t.Logf("empty keyword → %v", err)
	if err != nil {
		t.Fatalf("空关键词不该命中一切, got %v", err)
	}
}

func TestDefaultStatusRule_空体与有体(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    []byte
		wantErr bool
	}{
		{"空体404", 404, nil, true},
		{"空切片404", 404, []byte{}, true},
		{"1字节放行", 400, []byte("{"), false},
		{"有体放行", 500, []byte(`{"e":1}`), false},
		{"2xx空体仍报错(纯函数不管谁调用)", 200, nil, true},
	}
	for _, c := range cases {
		err := httpx.DefaultStatusRule(c.status, c.body)
		t.Logf("%s status=%d len(body)=%d err=%v", c.name, c.status, len(c.body), err)
		if !c.wantErr {
			if err != nil {
				t.Errorf("%s: 有体应放行，got %v", c.name, err)
			}
			continue
		}
		var se *kiterrors.HTTPStatusError
		if !errors.As(err, &se) || se.StatusCode != c.status {
			t.Errorf("%s: 要 *HTTPStatusError{%d}，got %v", c.name, c.status, err)
		}
	}
}

func TestRetryableTextRule_空体命中状态码As字段(t *testing.T) {
	rule := httpx.RetryableTextRule(nil, 503)
	err := rule(503, nil)
	t.Logf("503 空体 → %v", err)
	var re *kiterrors.RetryableError
	if !errors.As(err, &re) {
		t.Fatalf("要 *RetryableError，got %v", err)
	}
	var se *kiterrors.HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 503 {
		t.Fatalf("应包装 HTTPStatusError(503)，got %v", err)
	}
}
