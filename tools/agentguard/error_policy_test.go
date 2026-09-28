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
const opF = "sample.f"
var kind = kiterrors.NewKind("sample.bad")
func f(err error) error { return &kiterrors.Error{Op: opF, Kind: kind, Err: err} }
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

func TestFindDirectErrorConstructors_Op须引用具名常量(t *testing.T) {
	src := []byte(`package sample
import kit "github.com/japansms40-web/gohttpkit/errors"
const opF = "sample.f"
func f(err error) error {
  _ = kit.Error{Op: "sample.literal", Err: err}
  _ = &kit.Error{Op: "sample." + "concat", Err: err}
  return &kit.Error{Op: opF, Err: err}
}
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !strings.Contains(got[0], "sample.go:5:") || !strings.Contains(got[1], "sample.go:6:") {
		t.Fatalf("应只拦截第 5、6 行的非具名 Op，got %v", got)
	}
	for _, v := range got {
		if !strings.Contains(v, "Op 须引用具名 const") {
			t.Fatalf("违规说明不对：%v", got)
		}
	}
}

func TestFindDirectErrorConstructors_NewKind只能在包级var(t *testing.T) {
	src := []byte(`package sample
import kiterrors "github.com/japansms40-web/gohttpkit/errors"
var (
  KindA = kiterrors.NewKind("sample.a")
  kindB = kiterrors.NewKind("sample.sub.b_c")
)
var kinds = []kiterrors.Kind{kiterrors.NewKind("sample.nested")}
func f() bool {
  k := kiterrors.NewKind("sample.inline")
  return kiterrors.IsKind(nil, k) || kiterrors.IsKind(nil, kiterrors.NewKind("sample.arg"))
}
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	if len(got) != 3 || !strings.Contains(joined, "sample.go:7:") || !strings.Contains(joined, "sample.go:9:") ||
		!strings.Contains(joined, "sample.go:10:") || strings.Contains(joined, "sample.go:4:") {
		t.Fatalf("应拦截第 7、9、10 行（嵌套 / 函数体内），放行包级 var，got %v", got)
	}
	if !strings.Contains(joined, "NewKind 只能在包级 var 声明") {
		t.Fatalf("违规说明不对：%v", got)
	}
}

func TestFindDirectErrorConstructors_Kind名须小写点分(t *testing.T) {
	src := []byte(`package sample
import kiterrors "github.com/japansms40-web/gohttpkit/errors"
const name = "sample.const"
var (
  kOK      = kiterrors.NewKind("sample.ok_1")
  kNoDot   = kiterrors.NewKind("sample")
  kUpper   = kiterrors.NewKind("Sample.Bad")
  kSpace   = kiterrors.NewKind("sample.bad name")
  kEmpty   = kiterrors.NewKind("")
  kTrail   = kiterrors.NewKind("sample.")
  kIdent   = kiterrors.NewKind(name)
  kNoArg   = kiterrors.NewKind()
)
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("除 sample.ok_1 外 7 处都应拦截，got %v", got)
	}
	for _, v := range got {
		if !strings.Contains(v, "Kind 名须为 <包>.<分类> 小写点分字符串字面量") || strings.Contains(v, "sample.go:5:") {
			t.Fatalf("违规说明或行号不对：%v", got)
		}
	}
}

func TestFindDirectErrorConstructors_只认gohttpkit的errors包(t *testing.T) {
	src := []byte(`package sample
import (
  other "example.com/other/errors"
  kiterrors "github.com/japansms40-web/gohttpkit/errors"
)
type Error struct{ Op string }
var _ = Error{Op: "local.literal"}
var _ = other.Error{Op: "other literal"}
func f() { _ = other.NewKind("Whatever") }
var _ = kiterrors.KindOf(nil)
`)
	got, err := findDirectErrorConstructors("sample.go", src)
	if err != nil || len(got) != 0 {
		t.Fatalf("非 gohttpkit/errors 的同名符号不应拦截：violations=%v err=%v", got, err)
	}
}

func TestFindDirectErrorConstructors_Kind与Op规则测试文件豁免(t *testing.T) {
	got, err := findDirectErrorConstructors("fixture_test.go", []byte(`package sample
import kiterrors "github.com/japansms40-web/gohttpkit/errors"
func f() error { return &kiterrors.Error{Op: "x", Kind: kiterrors.NewKind("Bad")} }`))
	if err != nil || len(got) != 0 {
		t.Fatalf("测试文件应豁免：violations=%v err=%v", got, err)
	}
}
