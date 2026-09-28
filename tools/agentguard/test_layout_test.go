package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestScanTestLayout_合规仓库无违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/a.go":                         "package pkg\nfunc A() {}\n",
		"pkg/a_test.go":                    "package pkg\nfunc TestA(t *testing.T) {}\n",
		"pkg/doc.go":                       "// Package pkg 示例。\npackage pkg\n",
		"pkg/errors.go":                    "package pkg\nconst opA = \"pkg.a\"\nvar KindA = 1\ntype E struct{}\n",
		"pkg/iface.go":                     "package pkg\ntype I interface{ M() }\nfunc stub()\n",
		"pkg/characterization_test.go":     "package pkg\nfunc TestChar(t *testing.T) {}\n",
		"pkg/helpers_test.go":              "package pkg\nfunc newFixture() int { return 1 }\nfunc Testing() {}\n",
		"pkg/export_test.go":               "package pkg\nvar ExportA = A\n",
		"pkg/testdata/gen.go":              "package gen\nfunc G() {}\n",
		"pkg/testdata/fuzz/orphan_test.go": "package gen\n",
	})
	got, err := scanTestLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("合规布局不应报违规，得到 %v", got)
	}
}

func TestScanTestLayout_测试文件无同名源文件报违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/a.go":           "package pkg\nfunc A() {}\n",
		"pkg/a_test.go":      "package pkg\n",
		"pkg/a_more_test.go": "package pkg\n",
		"pkg/a_fuzz_test.go": "package pkg\n",
	})
	got, err := scanTestLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", got)
	want := []string{"pkg/a_fuzz_test.go", "pkg/a_more_test.go"}
	if len(got) != len(want) {
		t.Fatalf("期望 %d 条违规，得到 %v", len(want), got)
	}
	for i, path := range want {
		if !strings.HasPrefix(got[i], path+": ") || !strings.Contains(got[i], "pkg/"+strings.TrimSuffix(filepath.Base(path), "_test.go")+".go") {
			t.Fatalf("第 %d 条应指出 %s 缺少同名源文件，得到 %q", i, path, got[i])
		}
	}
}

func TestScanTestLayout_源文件有函数但无同名测试报违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/a.go":          "package pkg\nfunc A() {}\n",
		"pkg/m.go":          "package pkg\ntype T struct{}\nfunc (T) M() {}\n",
		"pkg/decl.go":       "package pkg\nvar V = func() int { return 1 }()\n",
		"pkg/other.go":      "package pkg\nfunc B() {}\n",
		"pkg/other_test.go": "package pkg\n",
	})
	got, err := scanTestLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", got)
	want := []string{"pkg/a.go: ", "pkg/m.go: "}
	if len(got) != len(want) {
		t.Fatalf("函数与方法都应要求同名测试，纯声明文件豁免；得到 %v", got)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(got[i], prefix) || !strings.Contains(got[i], strings.TrimSuffix(strings.TrimSuffix(prefix, ": "), ".go")+"_test.go") {
			t.Fatalf("第 %d 条应指出缺少 %s 的同名测试，得到 %q", i, prefix, got[i])
		}
	}
}

func TestScanTestLayout_豁免文件声明测试函数报违规(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/helpers_test.go":          "package pkg\nfunc TestHidden(t *testing.T) {}\n",
		"pkg/export_test.go":           "package pkg\nfunc FuzzX(f *testing.F) {}\nfunc Benchmark(b *testing.B) {}\nfunc ExampleA() {}\n",
		"pkg/characterization_test.go": "package pkg\nfunc TestChar(t *testing.T) {}\n",
		"pkg/kind_test.go":             "package pkg\ntype T struct{}\nfunc (T) TestMethod() {}\n",
		"pkg/kind.go":                  "package pkg\n",
	})
	got, err := scanTestLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", got)
	names := []string{"TestHidden", "FuzzX", "Benchmark", "ExampleA"}
	if len(got) != len(names) {
		t.Fatalf("helpers/export 里的每个测试函数都应报，characterization 与方法不算；得到 %v", got)
	}
	for _, name := range names {
		if !slices.ContainsFunc(got, func(v string) bool { return strings.Contains(v, name) }) {
			t.Fatalf("应报 %s，得到 %v", name, got)
		}
	}
}

func TestScanTestLayout_未跟踪文件也扫描忽略文件不扫描(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/a.go":      "package pkg\n",
		"pkg/a_test.go": "package pkg\n",
		".gitignore":    "ignored_test.go\n",
	})
	for name, src := range map[string]string{
		"pkg/a_more_test.go":  "package pkg\n",
		"pkg/ignored_test.go": "package pkg\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanTestLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "pkg/a_more_test.go: ") {
		t.Fatalf("应只报未跟踪的 a_more_test.go，得到 %v", got)
	}
}

func TestScanTestLayout_已删除的跟踪文件不参与判定(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/a.go":      "package pkg\nfunc A() {}\n",
		"pkg/a_test.go": "package pkg\n",
	})
	if err := os.Remove(filepath.Join(root, "pkg/a_test.go")); err != nil {
		t.Fatal(err)
	}
	got, err := scanTestLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "pkg/a.go: ") {
		t.Fatalf("删掉的测试文件不能算数，应报 a.go 缺测试，得到 %v", got)
	}
}

func TestScanTestLayout_源码解析失败返回错误(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"pkg/a.go": "package pkg\nfunc A( {\n",
	})
	if _, err := scanTestLayout(root); err == nil {
		t.Fatal("源码无法解析时必须返回错误，不能静默放行")
	}
	root2 := t.TempDir()
	initRepo(t, root2, map[string]string{
		"pkg/helpers_test.go": "package pkg\nfunc (\n",
	})
	if _, err := scanTestLayout(root2); err == nil {
		t.Fatal("豁免测试文件无法解析时必须返回错误")
	}
}

func TestScanTestLayout_不在仓库内返回错误(t *testing.T) {
	var pathErr *os.PathError
	_, err := scanTestLayout(t.TempDir())
	if err == nil {
		t.Fatal("非 git 目录应返回错误")
	}
	if !errors.As(err, &pathErr) || pathErr.Op != "git ls-files" {
		t.Fatalf("应返回 git ls-files 的 *os.PathError，得到 %T %v", err, err)
	}
}

func TestIsTestFuncName_按go_test规则识别(t *testing.T) {
	cases := map[string]bool{
		"Test": true, "TestA": true, "Test_中文": true, "Fuzz": true, "FuzzX": true,
		"Benchmark": true, "BenchmarkY": true, "Example": true, "ExampleA_b": true,
		"Testing": false, "Fuzzy": false, "Benchmarks": false, "Examples": false, "helper": false, "test": false,
	}
	for name, want := range cases {
		if got := isTestFuncName(name); got != want {
			t.Errorf("isTestFuncName(%q)=%v，期望 %v", name, got, want)
		}
	}
}
