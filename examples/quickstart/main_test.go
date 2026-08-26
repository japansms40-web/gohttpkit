package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRun_打通一次请求(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "gohttpkit-quickstart/1.0" {
			t.Errorf("UA = %q", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := run(&out, []string{"-url", srv.URL + "/get"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), `{"ok":true}`) {
		t.Fatalf("out = %q", out.String())
	}
}

func TestRun_参数与错误分支(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"非法 flag", []string{"-nope"}},
		{"URL 缺 scheme", []string{"-url", "example.com/x"}},
		{"URL 无法解析", []string{"-url", "://bad"}},
		{"代理 scheme 不支持", []string{"-url", "https://x.example/a", "-proxy", "ftp://h:1"}},
		{"请求失败", []string{"-url", "http://127.0.0.1:1/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(&out, tc.args); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestPreview_超长截断(t *testing.T) {
	if got := preview("  abc  ", 10); got != "abc" {
		t.Fatalf("got %q", got)
	}
	got := preview(strings.Repeat("x", 20), 5)
	if !strings.HasPrefix(got, "xxxxx\n") || !strings.Contains(got, "共 20 字节") {
		t.Fatalf("got %q", got)
	}
}

func TestMain_入口可直接跑(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	oldArgs := os.Args
	os.Args = []string{"quickstart", "-url", srv.URL + "/x"}
	t.Cleanup(func() { os.Args = oldArgs })
	main() // 成功路径不会 os.Exit
}
