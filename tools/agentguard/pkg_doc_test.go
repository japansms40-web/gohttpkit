package main

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// pkgDocWithTree 拼一个带文件结构树的 doc.go；entries 形如 "a.go  说明"。
func pkgDocWithTree(pkg string, entries ...string) string {
	var b strings.Builder
	b.WriteString("// Package " + pkg + " 示例。\n//\n// 文件结构：\n//\n//\t" + pkg + "/\n")
	for i, e := range entries {
		branch := "├── "
		if i == len(entries)-1 {
			branch = "└── "
		}
		b.WriteString("//\t" + branch + e + "\n")
	}
	b.WriteString("package " + pkg + "\n")
	return b.String()
}

func TestScanPkgDoc_合规仓库无违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/doc.go":            pkgDocWithTree("pkg", "a.go      入口", "doc.go    包文档（本文件）", "decl.go   纯声明", "sub/      子包", "group/    分组目录"),
		"pkg/a.go":              "package pkg\n\n// A 说明。\nfunc A() {}\n",
		"pkg/a_test.go":         "// Package pkg 测试文件的注释不受约束。\npackage pkg\n",
		"pkg/decl.go":           "package pkg\n\n// V 说明。\nvar V = 1\n",
		"pkg/sub/doc.go":        pkgDocWithTree("sub", "doc.go  包文档"),
		"pkg/group/leaf/doc.go": pkgDocWithTree("leaf", "doc.go  包文档"),
		"pkg/testdata/x.go":     "package x\n",
		"pkg/testdata/y/y.go":   "package y\n",
		"onlytest/a_test.go":    "package onlytest\n",
	})
	got, err := scanPkgDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("合规布局不应报违规（testdata、只有测试文件的目录、无 Go 文件的中间目录都不要求 doc.go），得到 %v", got)
	}
}

func TestScanPkgDoc_缺doc报违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"main.go":    "package main\n\nfunc main() {}\n",
		"pkg/a.go":   "package pkg\n",
		"pkg/b/b.go": "package b\n",
		"pkg/doc.go": pkgDocWithTree("pkg", "a.go  入口", "doc.go  包文档", "b/  子包"),
	})
	got, err := scanPkgDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", got)
	want := []string{"doc.go: 缺少 doc.go", "pkg/b/doc.go: 缺少 doc.go"}
	if len(got) != len(want) {
		t.Fatalf("根目录包与子包都应报缺 doc.go，得到 %v", got)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(got[i], prefix) {
			t.Fatalf("第 %d 条应以 %q 开头，得到 %q", i, prefix, got[i])
		}
	}
}

func TestScanPkgDoc_doc无包注释或无文件树报违规(t *testing.T) {
	cases := map[string]struct {
		doc  string
		want string
	}{
		"无包注释":      {"package pkg\n", "pkg/doc.go: 缺少包注释"},
		"注释与包子句隔空行": {"// Package pkg 示例。\n\npackage pkg\n", "pkg/doc.go: 缺少包注释"},
		"无文件树标题":    {"// Package pkg 示例。\n//\n//\t├── doc.go  包文档\npackage pkg\n", "pkg/doc.go: 包注释缺少「文件结构：」文件树"},
		"标题下无条目":    {"// Package pkg 示例。\n//\n// 文件结构：\n//\n//\tpkg/\npackage pkg\n", "pkg/doc.go: 包注释缺少「文件结构：」文件树"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			initRepo(t, root, map[string]string{"pkg/doc.go": c.doc})
			got, err := scanPkgDoc(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%v", got)
			if len(got) != 1 || !strings.HasPrefix(got[0], c.want) {
				t.Fatalf("应只报 %q，得到 %v", c.want, got)
			}
		})
	}
}

func TestScanPkgDoc_包注释写在非doc文件报违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/doc.go":    pkgDocWithTree("pkg", "a.go  入口", "b.go  其它", "doc.go  包文档"),
		"pkg/a.go":      "// Package pkg 重复的包注释。\npackage pkg\n",
		"pkg/b.go":      "// 普通注释与包子句隔一行，不算包注释。\n\npackage pkg\n",
		"pkg/a_test.go": "package pkg\n",
	})
	got, err := scanPkgDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", got)
	if len(got) != 1 || !strings.HasPrefix(got[0], "pkg/a.go: 包注释只能写在 doc.go") || !strings.Contains(got[0], "pkg/doc.go") {
		t.Fatalf("应只报 a.go 的包注释位置违规并指向 pkg/doc.go，得到 %v", got)
	}
}

func TestScanPkgDoc_文件树与实际不一致报违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/doc.go": pkgDocWithTree("pkg",
			"doc.go    包文档",
			"a.go      入口",
			"a.go      重复登记",
			"gone.go   已删除的文件",
			"nodesc.go",
			"old/      已删除的子目录",
			"a_test.go 测试文件不应登记",
		),
		"pkg/a.go":          "package pkg\n",
		"pkg/nodesc.go":     "package pkg\n",
		"pkg/new.go":        "package pkg\n",
		"pkg/a_test.go":     "package pkg\n",
		"pkg/sub/doc.go":    pkgDocWithTree("sub", "doc.go  包文档"),
		"pkg/sub/x_test.go": "package sub\n",
	})
	got, err := scanPkgDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", got)
	want := []string{
		"pkg/doc.go: 文件结构树重复登记 a.go",
		"pkg/doc.go: 文件结构树条目 nodesc.go 缺少用途说明",
		"pkg/doc.go: 文件结构树登记了不存在的 a_test.go",
		"pkg/doc.go: 文件结构树登记了不存在的 gone.go",
		"pkg/doc.go: 文件结构树登记了不存在的 old/",
		"pkg/doc.go: 文件结构树未登记 new.go",
		"pkg/doc.go: 文件结构树未登记 sub/",
	}
	for _, w := range want {
		if !slices.ContainsFunc(got, func(g string) bool { return strings.HasPrefix(g, w) }) {
			t.Errorf("缺少违规 %q", w)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 条违规，得到 %d 条：%v", len(want), len(got), got)
	}
	if !slices.IsSorted(got) {
		t.Fatalf("违规应排序输出，得到 %v", got)
	}
}

func TestScanPkgDoc_嵌套树行不算一层条目(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/doc.go":      pkgDocWithTree("pkg", "doc.go  包文档", "sub/    子包\n//\t│   └── deep.go  下一层由子包自己的 doc.go 登记"),
		"pkg/sub/doc.go":  pkgDocWithTree("sub", "deep.go  深层", "doc.go  包文档"),
		"pkg/sub/deep.go": "package sub\n",
	})
	got, err := scanPkgDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("以 │ 缩进的下层行不应算作本包条目，得到 %v", got)
	}
}

func TestScanPkgDoc_工作区已删除的跟踪文件不算(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/doc.go": pkgDocWithTree("pkg", "doc.go  包文档"),
		"pkg/a.go":   "package pkg\n",
		"old/a.go":   "package old\n",
	})
	for _, rel := range []string{"pkg/a.go", "old/a.go"} {
		if err := os.Remove(root + "/" + rel); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanPkgDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("已删除的文件不应要求登记，其目录也不再是 Go 包，得到 %v", got)
	}
}

func TestScanPkgDoc_源码解析失败返回错误(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"普通源文件":  {"pkg/doc.go": pkgDocWithTree("pkg", "a.go  入口", "doc.go  包文档"), "pkg/a.go": "package pkg\nfunc A( {\n"},
		"doc.go": {"pkg/doc.go": "// Package pkg\npackage pkg\nfunc (\n"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			initRepo(t, root, files)
			if _, err := scanPkgDoc(root); err == nil {
				t.Fatal("源码无法解析时必须返回错误，不能静默放行")
			}
		})
	}
}

func TestScanPkgDoc_不在仓库内返回错误(t *testing.T) {
	var pathErr *os.PathError
	_, err := scanPkgDoc(t.TempDir())
	if !errors.As(err, &pathErr) || pathErr.Op != "git ls-files" {
		t.Fatalf("不在 Git 仓库内应返回 git ls-files 的 *os.PathError，得到 %v", err)
	}
}
