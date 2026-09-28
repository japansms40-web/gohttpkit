package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// testLayoutExempt 是不对应单个源文件、允许独立存在的测试文件名（CODE_STANDARDS §8）。
// characterization_test.go 是跨切面的行为锁；helpers_test.go 只放共享夹具；export_test.go 是给外部测试包的内部导出桥。
var testLayoutExempt = map[string]bool{
	"characterization_test.go": true,
	"helpers_test.go":          true,
	"export_test.go":           true,
}

// testFuncPrefixes 是 go test 识别的测试函数前缀。
var testFuncPrefixes = []string{"Test", "Fuzz", "Benchmark", "Example"}

// scanTestLayout 检查测试文件与源文件一一对应（CODE_STANDARDS §8）。
// 输入 root 是 Git 仓库根目录；扫描已跟踪与未跟踪（不含忽略）的 Go 文件，含独立子模块，跳过 testdata/。
// 规则：
//   - 每个 X_test.go 必须有同目录 X.go（豁免 testLayoutExempt）；
//   - helpers_test.go / export_test.go 不得声明测试函数；
//   - 声明了函数或方法的 X.go 必须有同目录 X_test.go。
//
// 返回排序后的全部违规；列文件、读文件或解析失败时返回错误，不能静默放行。
func scanTestLayout(root string) ([]string, error) {
	listed, err := gitRaw(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.go")
	if err != nil {
		return nil, &os.PathError{Op: "git ls-files", Path: root, Err: err}
	}
	present := make(map[string]bool)
	for _, rel := range strings.Split(listed, "\x00") {
		if rel == "" || inTestdata(rel) {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(root, rel)); statErr != nil {
			if os.IsNotExist(statErr) {
				continue // 工作区中已删除的跟踪文件
			}
			return nil, statErr
		}
		present[rel] = true
	}
	var violations []string
	for rel := range present {
		dir, base := path.Split(rel)
		if stem, isTest := strings.CutSuffix(base, "_test.go"); isTest {
			found, checkErr := checkTestFile(root, rel, dir+stem+".go", present)
			if checkErr != nil {
				return nil, checkErr
			}
			violations = append(violations, found...)
			continue
		}
		want := strings.TrimSuffix(rel, ".go") + "_test.go"
		if present[want] {
			continue
		}
		hasFunc, parseErr := declaresFunc(root, rel)
		if parseErr != nil {
			return nil, parseErr
		}
		if hasFunc {
			violations = append(violations, rel+": 声明了函数但缺少同名测试文件 "+want+"，用例须写在该文件里")
		}
	}
	sort.Strings(violations)
	return violations, nil
}

// checkTestFile 检查一个测试文件：普通测试文件要有同名源文件，豁免文件不得声明测试函数。
func checkTestFile(root, rel, source string, present map[string]bool) ([]string, error) {
	base := path.Base(rel)
	if !testLayoutExempt[base] {
		if present[source] {
			return nil, nil
		}
		return []string{rel + ": 测试文件须与同目录源文件同名（缺少 " + source + "），用例请并入被测源文件对应的 _test.go"}, nil
	}
	if base == "characterization_test.go" {
		return nil, nil
	}
	f, err := parseGoFile(root, rel)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && isTestFuncName(fn.Name.Name) {
			out = append(out, rel+": "+base+" 只放夹具，不得声明测试函数 "+fn.Name.Name+"，请移到被测源文件对应的 _test.go")
		}
	}
	return out, nil
}

// declaresFunc 报告源文件是否声明了带函数体的函数或方法；doc.go 与纯 const/var/type 文件返回 false。
func declaresFunc(root, rel string) (bool, error) {
	f, err := parseGoFile(root, rel)
	if err != nil {
		return false, err
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			return true, nil
		}
	}
	return false, nil
}

func parseGoFile(root, rel string) (*ast.File, error) {
	src, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, &os.PathError{Op: "read", Path: rel, Err: err}
	}
	return parser.ParseFile(token.NewFileSet(), rel, src, parser.SkipObjectResolution)
}

// isTestFuncName 按 go test 规则判断：前缀后为空或首字符不是小写字母（TestA、Test_x 算，Testing 不算）。
func isTestFuncName(name string) bool {
	for _, prefix := range testFuncPrefixes {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if rest == "" {
			return true
		}
		r, _ := utf8.DecodeRuneInString(rest)
		return !unicode.IsLower(r)
	}
	return false
}

func inTestdata(rel string) bool {
	return rel == "testdata" || strings.HasPrefix(rel, "testdata/") || strings.Contains(rel, "/testdata/")
}
