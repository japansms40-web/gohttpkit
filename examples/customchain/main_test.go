package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestMain_整例跑通 直接把示例跑一遍：它自带假服务器，不需要外网。
// 示例是给人读的，但读者会照抄——所以它必须真的能跑，且行为如注释所述。
func TestMain_整例跑通(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("示例跑挂了: %v", r)
		}
	}()
	main()
}

func TestSessionHeaders_回写后下次请求带新值(t *testing.T) {
	h := &sessionHeaders{base: "https://x.example", token: "t0"}
	if got := h.BuildHeaders(context.Background())["authorization"]; got != "Bearer t0" {
		t.Fatalf("got %q", got)
	}
	h.setToken("t1")
	if got := h.BuildHeaders(context.Background())["authorization"]; got != "Bearer t1" {
		t.Fatalf("回写后 = %q", got)
	}
	if h.BaseURL() != "https://x.example" {
		t.Fatal("BaseURL 不对")
	}
}

func TestSessionHeaders_并发读写(t *testing.T) {
	h := &sessionHeaders{base: "https://x.example", token: "t0"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			h.setToken("t")
		}
	}()
	for i := 0; i < 500; i++ {
		if got := h.BuildHeaders(context.Background())["authorization"]; got == "" {
			t.Fatal("并发读到了空 authorization——构头器的锁没护住")
		}
	}
	<-done
}

func TestErrAccountBanned_可被errorsIs识别(t *testing.T) {
	wrapped := errors.Join(ErrAccountBanned, errors.New("extra"))
	if !errors.Is(wrapped, ErrAccountBanned) {
		t.Fatal("sentinel 应可被 errors.Is 识别")
	}
}

func TestFakeServer_封禁路径返回200但业务失败(t *testing.T) {
	srv := fakeServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/banned")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, 这个例子的重点就是 HTTP 200 + 业务失败", resp.StatusCode)
	}

	resp2, err := http.Get(srv.URL + "/api/profile")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if got := resp2.Header.Get("x-refresh-token"); !strings.HasPrefix(got, "token-") {
		t.Fatalf("x-refresh-token = %q", got)
	}
}
