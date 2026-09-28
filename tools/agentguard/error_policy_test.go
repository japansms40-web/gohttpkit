package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDirectErrorConstructors_识别别名与字符串panic(t *testing.T) {
	src := []byte(`package sample
import (
  stderrors "errors"
  formatter "fmt"
)
var cause = stderrors.New("boom")
func f(err error) error { return formatter.Errorf("wrap: %w", err) }
func g() { panic(formatter.Sprintf("bad: %d", 1)); panic("bad") }
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"errors.New", "fmt.Errorf", "panic(fmt.Sprintf)", "panic(string)"} {
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Fatalf("未检出 %s：%v", want, got)
		}
	}
}

func TestFindDirectErrorConstructors_拦截间接字符串panic(t *testing.T) {
	src := []byte(`package sample
import "strings"
func f() {
  msg := "bad"
  panic("a" + "b")
  panic(msg)
  panic(strings.Join([]string{"a", "b"}, ""))
}
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("应拦截三处非类型错误 panic，got %v", got)
	}
}

func TestFindDirectErrorConstructors_放行类型错误panic(t *testing.T) {
	src := []byte(`package sample
type LocalError struct{}
func (*LocalError) Error() string { return "bad" }
func f(err error) { panic(err); panic(&LocalError{}) }
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil || len(got) != 0 {
		t.Fatalf("类型错误 panic 应放行：violations=%v err=%v", got, err)
	}
}

func TestScanErrorPolicy_覆盖主模块子模块和未跟踪生产文件(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, map[string]string{
		"main.go":                   "package sample\nimport \"errors\"\nvar x = errors.New(\"x\")\n",
		"fixture_test.go":           "package sample\nimport \"errors\"\nvar x = errors.New(\"x\")\n",
		"tools/agentguard/guard.go": "package main\nimport \"fmt\"\nvar x = fmt.Errorf(\"x\")\n",
		".gitignore":                "ignored.go\n",
	})
	for name, src := range map[string]string{
		"extra.go":   "package sample\nfunc f() { panic(\"x\") }\n",
		"ignored.go": "package sample\nimport \"errors\"\nvar x = errors.New(\"x\")\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanErrorPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("应拦主模块、子模块和未跟踪文件，忽略测试与忽略文件；got %v", got)
	}
}

func TestFindDirectErrorConstructors_放行结构化错误与普通格式化(t *testing.T) {
	src := []byte(`package sample
import (
  "fmt"
  kiterrors "github.com/japansms40-web/gohttpkit/errors"
)
var kind = kiterrors.NewKind("sample.bad")
func f(err error) error { return &kiterrors.Error{Op: "sample.f", Kind: kind, Err: err} }
func message() string { return fmt.Sprintf("err=%v", f(nil)) }
// fmt.Errorf("comment")
var example = "errors.New(\"string\")"
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil || len(got) != 0 {
		t.Fatalf("合法源码被拦截：violations=%v err=%v", got, err)
	}
}

func TestFindDirectErrorConstructors_测试文件豁免与语法错误(t *testing.T) {
	got, err := findDirectErrorConstructors("fixture_test.go", []byte(`package sample
import "errors"
var cause = errors.New("fixture")`))
	if err != nil || len(got) != 0 {
		t.Fatalf("测试夹具应豁免：violations=%v err=%v", got, err)
	}
	if _, err := findDirectErrorConstructors("broken.go", []byte("package sample\nfunc (")); err == nil {
		t.Fatal("生产源码解析失败应阻断")
	}
}
