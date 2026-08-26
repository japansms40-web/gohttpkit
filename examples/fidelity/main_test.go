package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// TestRun_整例跑通含落盘 示例自带假服务器，落盘目录用 t.TempDir。
func TestRun_整例跑通含落盘(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := run(&out, []string{"-dump", dir}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "got_csrf") {
		t.Fatalf("out = %q", out.String())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("落盘文件数 = %d, want 2", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, "step002_transaction.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tx struct {
		Method     string              `json:"method"`
		Status     int                 `json:"status"`
		ReqHeaders map[string][]string `json:"req_headers"`
	}
	if err := json.Unmarshal(data, &tx); err != nil {
		t.Fatal(err)
	}
	if tx.Method != http.MethodPost || tx.Status != 200 {
		t.Fatalf("tx = %+v", tx)
	}
	// 快照必须如实：不含用于抑制标准库默认 UA 的空条目，且带上 host
	if _, bad := tx.ReqHeaders["User-Agent"]; bad {
		t.Fatal("快照混进了规范化 User-Agent")
	}
	if len(tx.ReqHeaders["host"]) == 0 {
		t.Fatal("快照缺 host")
	}
}

func TestRun_不落盘也能跑(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRun_非法参数报错(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, []string{"-nope"}); err == nil {
		t.Fatal("want error")
	}
}

func TestBuildCookie_按抓包顺序拼且跳过空值(t *testing.T) {
	// cookie 顺序是指纹的一部分：map 遍历顺序随机，必须按 cookieOrder 固定下来。
	h := &browserHeaders{
		base:      "https://x.example",
		csrfToken: "c1",
		sessionID: "s1",
		extraCk:   map[string]string{"datr": "d1", "mid": "m1"},
	}
	got := h.BuildHeaders(context.Background())["cookie"]
	want := "datr=d1; mid=m1; csrftoken=c1; sessionid=s1"
	if got != want {
		t.Fatalf("cookie = %q, want %q", got, want)
	}

	// 空值字段整条不发，而不是发一个 name=
	h2 := &browserHeaders{base: "https://x.example", csrfToken: "c1"}
	if got := h2.BuildHeaders(context.Background())["cookie"]; got != "csrftoken=c1" {
		t.Fatalf("cookie = %q, 空值字段应整条跳过", got)
	}

	// 未出现在 cookieOrder 里的字段不发（顺序未知的字段宁可不发）
	h3 := &browserHeaders{base: "https://x.example", extraCk: map[string]string{"unknown": "u"}}
	if got := h3.BuildHeaders(context.Background())["cookie"]; got != "" {
		t.Fatalf("cookie = %q, 未登记顺序的字段不该发", got)
	}
}

func TestApplySetCookie_回写三类字段(t *testing.T) {
	h := &browserHeaders{base: "https://x.example"}
	resp := http.Header{}
	resp.Add("Set-Cookie", "csrftoken=new-csrf; Path=/; Secure")
	resp.Add("Set-Cookie", "sessionid=new-sess; HttpOnly")
	resp.Add("Set-Cookie", "datr=new-datr")
	resp.Add("Set-Cookie", "empty=; Path=/") // 空值不回写
	resp.Add("Set-Cookie", "malformed")      // 无 = 号，跳过

	h.applySetCookie(resp)

	if h.csrfToken != "new-csrf" || h.sessionID != "new-sess" {
		t.Fatalf("h = %+v", h)
	}
	if h.extraCk["datr"] != "new-datr" {
		t.Fatalf("extraCk = %v", h.extraCk)
	}
	if _, bad := h.extraCk["empty"]; bad {
		t.Fatal("空值不该被回写")
	}
	if _, bad := h.extraCk["malformed"]; bad {
		t.Fatal("畸形条目不该被回写")
	}
}

func TestParseSetCookie(t *testing.T) {
	cases := []struct {
		raw         string
		name, value string
		ok          bool
	}{
		{"a=1; Path=/", "a", "1", true},
		{"  a  =  1  ", "a", "1", true},
		{"noequals", "noequals", "", false},
	}
	for _, tc := range cases {
		name, value, ok := parseSetCookie(tc.raw)
		if ok != tc.ok || (tc.ok && (name != tc.name || value != tc.value)) {
			t.Fatalf("parseSetCookie(%q) = %q,%q,%v", tc.raw, name, value, ok)
		}
	}
}

func TestBuildHeaders_头名全小写(t *testing.T) {
	h := &browserHeaders{base: "https://x.example"}
	for k := range h.BuildHeaders(context.Background()) {
		if k != strings.ToLower(k) {
			t.Fatalf("头名 %q 不是小写——大写形态是明显的机器特征", k)
		}
	}
}

func TestDump_目录不可写时不炸(t *testing.T) {
	// 落盘失败只该打一行 stderr，不该让整个流程崩掉 —— 调试辅助不能反过来搞挂主流程。
	tx := &httpx.Transaction{Method: http.MethodGet, URL: "https://x.example/a", Status: 200}
	var out bytes.Buffer

	// 目标是个已存在的文件而不是目录 → MkdirAll 失败
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dump(&out, file, 1, tx)

	// 目录建得出来，但文件名指向一个已存在的目录 → WriteFile 失败
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "step002_transaction.json"), 0o750); err != nil {
		t.Fatal(err)
	}
	dump(&out, dir, 2, tx)

	if strings.Count(out.String(), "落盘失败") != 2 {
		t.Fatalf("两次失败都该提示: %q", out.String())
	}
}

func TestPrintTransaction_不炸(t *testing.T) {
	var out bytes.Buffer
	printTransaction(&out, 1, &httpx.Transaction{
		Method:     http.MethodGet,
		URL:        "https://x.example/a",
		Status:     200,
		ReqHeaders: map[string][]string{"accept": {"*/*"}},
	})
	if !strings.Contains(out.String(), "snapshot 01") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestMain_入口可直接跑(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"fidelity"}
	t.Cleanup(func() { os.Args = oldArgs })
	main() // 成功路径不会 os.Exit
}
