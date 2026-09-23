package versionreg

// registry_internal_test.go —— 未导出 copyIDsLocked / sortIDs 的角度测试。
// 这两层是 List / 错误 Registered 快照的单一实现，只靠导出 API 测不到
// nil 切片、就地排序、空表容量这些分支。

import (
	"testing"
)

func TestCopyIDsLocked_空表是长度0的新切片(t *testing.T) {
	r := New[*Config]("t")
	r.mu.RLock()
	ids := r.copyIDsLocked()
	r.mu.RUnlock()
	t.Logf("空表 copyIDsLocked = %v nil=%v cap=%d", ids, ids == nil, cap(ids))
	if ids == nil || len(ids) != 0 {
		t.Fatalf("空表应返回长度 0 的切片（非 nil），得到 %v", ids)
	}
	injected := append(ids, "injected")
	if len(injected) != 1 || injected[0] != "injected" {
		t.Fatalf("append 结果应为单元素，得到 %v", injected)
	}
	r.mu.RLock()
	again := r.copyIDsLocked()
	r.mu.RUnlock()
	if len(again) != 0 {
		t.Fatal("append 返回切片不得写回 items")
	}
}

func TestCopyIDsLocked_单元素与多元素成员完整(t *testing.T) {
	r := New[*Config]("t")
	r.MustRegister(NewConfig("only", "https://a.example"))
	r.mu.RLock()
	one := r.copyIDsLocked()
	r.mu.RUnlock()
	t.Logf("单元素 = %v", one)
	if len(one) != 1 || one[0] != "only" {
		t.Fatalf("单元素 copy = %v", one)
	}

	r.MustRegister(NewConfig("a", "https://a.example"))
	r.MustRegister(NewConfig("z", "https://a.example"))
	r.mu.RLock()
	many := r.copyIDsLocked()
	r.mu.RUnlock()
	t.Logf("多元素（未排序）= %v", many)
	if len(many) != 3 {
		t.Fatalf("len=%d, want 3", len(many))
	}
	seen := map[ID]int{}
	for _, id := range many {
		seen[id]++
	}
	for _, want := range []ID{"only", "a", "z"} {
		if seen[want] != 1 {
			t.Fatalf("缺/重 %q: %v", want, many)
		}
	}
}

func TestCopyIDsLocked_两次调用互不共享底层数组(t *testing.T) {
	r := New[*Config]("t")
	r.MustRegister(NewConfig("v1", "https://a.example"))
	r.MustRegister(NewConfig("v2", "https://a.example"))
	r.mu.RLock()
	a := r.copyIDsLocked()
	b := r.copyIDsLocked()
	r.mu.RUnlock()
	a[0] = "mutated"
	t.Logf("改 a 后 b=%v", b)
	for _, id := range b {
		if id == "mutated" {
			t.Fatal("两次 copyIDsLocked 不得共享底层数组")
		}
	}
}

func TestSortIDs_nil与空切片不panic(t *testing.T) {
	sortIDs(nil)
	empty := []ID{}
	sortIDs(empty)
	t.Logf("nil / 空切片 sort 后 empty=%v", empty)
	if len(empty) != 0 {
		t.Fatalf("空切片 sort 后 len=%d", len(empty))
	}
}

func TestSortIDs_单元素保持(t *testing.T) {
	ids := []ID{"only"}
	sortIDs(ids)
	t.Logf("单元素 = %v", ids)
	if len(ids) != 1 || ids[0] != "only" {
		t.Fatalf("单元素被改成 %v", ids)
	}
}

func TestSortIDs_已排序保持_逆序排开(t *testing.T) {
	sorted := []ID{"a", "b", "c"}
	sortIDs(sorted)
	t.Logf("已排序 = %v", sorted)
	if sorted[0] != "a" || sorted[2] != "c" {
		t.Fatalf("已排序被打乱: %v", sorted)
	}

	rev := []ID{"c", "b", "a"}
	sortIDs(rev)
	t.Logf("逆序 → %v", rev)
	if rev[0] != "a" || rev[1] != "b" || rev[2] != "c" {
		t.Fatalf("逆序未排开: %v", rev)
	}
}

func TestSortIDs_重复键保持相邻且就地改(t *testing.T) {
	ids := []ID{"b", "a", "a", "c"}
	orig := ids
	sortIDs(ids)
	t.Logf("含重复 → %v", ids)
	want := []ID{"a", "a", "b", "c"}
	if len(ids) != 4 || ids[0] != "a" || ids[1] != "a" || ids[2] != "b" || ids[3] != "c" {
		t.Fatalf("got %v, want %v", ids, want)
	}
	if &ids[0] != &orig[0] {
		t.Fatal("sortIDs 应就地改同一切片")
	}
}

func TestSortIDs_字典序不是semver(t *testing.T) {
	ids := []ID{"v10", "v2", "v1"}
	sortIDs(ids)
	t.Logf("v10/v2/v1 → %v", ids)
	if ids[0] != "v1" || ids[1] != "v10" || ids[2] != "v2" {
		t.Fatalf("应是字典序 [v1 v10 v2]，得到 %v", ids)
	}
}
