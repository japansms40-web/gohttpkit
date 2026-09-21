package versionreg_test

// endpoints_more_test.go —— 按 docs/TESTING.md §2 补 Endpoint / HeaderWhitelists。
// 现有测试已锁 nil 安全、深拷贝、nil 源空集合；这里补「配置了空内层 ≠ 未配置」、
// 返回值隔离、空键、字典序边界。out-of-scope：资源生命周期（无可关对象）。

import (
	"fmt"
	"sync"
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func TestEndpoint_String边界与不做trim(t *testing.T) {
	cases := []struct {
		in   versionreg.Endpoint
		want string
	}{
		{"", ""},
		{" ", " "},
		{"user.profile", "user.profile"},
		{"User.Profile", "User.Profile"},
		{"用户.资料", "用户.资料"},
	}
	for _, c := range cases {
		got := c.in.String()
		t.Logf("Endpoint(%q).String() = %q", string(c.in), got)
		if got != c.want {
			t.Errorf("Endpoint(%q).String() = %q, want %q", string(c.in), got, c.want)
		}
	}
	if got := fmt.Sprint(versionreg.Endpoint("thread.send")); got != "thread.send" {
		t.Fatalf("fmt.Stringer = %q", got)
	}
}

func TestNewHeaderWhitelists_空map与nil源同为空集合(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{})
	t.Logf("空 map For=%v Has=%v Endpoints=%v", w.For("x"), w.Has("x"), w.Endpoints())
	if w == nil {
		t.Fatal("空 map 源也应返回可用的空集合")
	}
	if w.For("x") != nil || w.Has("x") {
		t.Fatal("空 map 源的 For/Has 应与 nil 源一样")
	}
	if w.Endpoints() == nil || len(w.Endpoints()) != 0 {
		t.Fatalf("空集合 Endpoints 长度应为 0，got %v", w.Endpoints())
	}
}

func TestHeaderWhitelists_配置空内层不是未配置(t *testing.T) {
	// 契约：未配置 → For 返回 nil（发全量头）；已配置空 map → For 返回空 map（一个头都不发）。
	// 这是反向推理挖出的隐藏角度：src 里 ep→{} 或 ep→nil 都会写入空内层。
	for name, inner := range map[string]map[string]string{
		"空map":  {},
		"nil内层": nil,
	} {
		t.Run(name, func(t *testing.T) {
			w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
				"user.profile": inner,
			})
			got := w.For("user.profile")
			t.Logf("%s For=%v nil=%v Has=%v", name, got, got == nil, w.Has("user.profile"))
			if !w.Has("user.profile") {
				t.Fatal("已配置（哪怕内层空）Has 应为 true")
			}
			if got == nil {
				t.Fatal("已配置空内层 For 必须是空 map，不能是 nil（nil = 发全量头）")
			}
			if len(got) != 0 {
				t.Fatalf("空内层 For 长度应为 0，got %v", got)
			}
			if w.For("zzz") != nil {
				t.Fatal("未配置的另一个 endpoint 仍应是 nil")
			}
		})
	}
}

func TestHeaderWhitelists_For副本隔离(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
		"user.profile": {"accept": "", "x-app-id": "123"},
	})
	got := w.For("user.profile")
	got["injected"] = "boom"
	got["accept"] = "mutated"
	again := w.For("user.profile")
	t.Logf("改副本后再次 For=%v", again)
	if _, leaked := again["injected"]; leaked {
		t.Fatal("For 必须返回新 map，就地改写不得污染集合")
	}
	if again["accept"] != "" {
		t.Fatal("改副本的已有键不得写回")
	}
	if again["x-app-id"] != "123" {
		t.Fatalf("未改的键应保持，got %v", again)
	}
}

func TestNewHeaderWhitelists_空端点与空头名(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
		"": {"": "fixed", "x": ""},
	})
	t.Logf("Has(\"\")=%v For=%v Endpoints=%v", w.Has(""), w.For(""), w.Endpoints())
	if !w.Has("") {
		t.Fatal("空 Endpoint 作为键应可配置")
	}
	got := w.For("")
	if got[""] != "fixed" || got["x"] != "" {
		t.Fatalf("空头名 / 空值应原样保留: %v", got)
	}
	if w.Has(" ") || w.For(" ") != nil {
		t.Fatal("空串端点与空白端点是不同的键")
	}
}

func TestHeaderWhitelists_Endpoints切片隔离与单元素(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
		"only": {"a": "1"},
	})
	one := w.Endpoints()
	t.Logf("单元素 Endpoints=%v", one)
	if len(one) != 1 || one[0] != "only" {
		t.Fatalf("Endpoints = %v", one)
	}
	one[0] = "mutated"
	again := w.Endpoints()
	t.Logf("改切片后再次 Endpoints=%v", again)
	if again[0] != "only" {
		t.Fatal("Endpoints 必须返回新切片")
	}
	if !w.Has("only") {
		t.Fatal("改返回切片不得删掉内部键")
	}
}

func TestHeaderWhitelists_Endpoints字典序含空白(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
		"z": {},
		"a": {},
		" ": {},
	})
	got := w.Endpoints()
	t.Logf("Endpoints = %v", got)
	if len(got) != 3 || got[0] != " " || got[1] != "a" || got[2] != "z" {
		t.Fatalf("Endpoints = %v, want 字典序 [\" \" a z]", got)
	}
}

func TestHeaderWhitelists_并发For副本互不干扰(t *testing.T) {
	w := versionreg.NewHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
		"user.profile": {"accept": "", "x-k": "v"},
	})
	const n = 8
	var wg sync.WaitGroup
	errCh := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got := w.For("user.profile")
				_, hasAccept := got["accept"]
				if got["x-k"] != "v" || !hasAccept {
					errCh <- fmt.Sprintf("goroutine %d 读到脏副本 %v", i, got)
					return
				}
				got["injected"] = fmt.Sprintf("%d-%d", i, j)
				if !w.Has("user.profile") || w.For("zzz") != nil {
					errCh <- fmt.Sprintf("goroutine %d 并发读坏了 Has/未配置", i)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
	clean := w.For("user.profile")
	t.Logf("并发后 For=%v", clean)
	if _, leaked := clean["injected"]; leaked {
		t.Fatal("并发改各自副本不得污染集合")
	}
}
