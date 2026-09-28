package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// findDirectErrorConstructors 检查生产 Go 源码的直接错误构造；测试夹具不受此规则限制。
// 输入 filename 用于判断测试文件和定位行号，src 是对应源码。
// 返回违规位置与解析错误；源码解析失败时调用方必须阻断。
func findDirectErrorConstructors(filename string, src []byte) ([]string, error) {
	if strings.HasSuffix(filename, "_test.go") {
		return nil, nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	imports := make(map[string]string)
	var violations []string
	add := func(pos token.Pos, rule string) {
		violations = append(violations, fmt.Sprintf("%s:%d: %s", filename, fset.Position(pos).Line, rule))
	}
	for _, imp := range f.Imports {
		path, unquoteErr := strconv.Unquote(imp.Path.Value)
		if unquoteErr != nil || path != "errors" && path != "fmt" {
			continue
		}
		name := path
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			add(imp.Pos(), "禁止点导入标准库 errors/fmt")
			continue
		}
		imports[name] = path
	}
	isStdCall := func(expr ast.Expr, path, name string) bool {
		sel, ok := expr.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && imports[id.Name] == path
	}
	// panic 只允许已有错误变量 err 或直接构造的 XxxError；其它表达式可能产生字符串值。
	// 这是语法门禁，err 的实际类型仍需调用点审查。
	isTypedPanicArg := func(expr ast.Expr) bool {
		if id, ok := expr.(*ast.Ident); ok {
			return id.Name == "err"
		}
		addr, ok := expr.(*ast.UnaryExpr)
		if !ok || addr.Op != token.AND {
			return false
		}
		literal, ok := addr.X.(*ast.CompositeLit)
		if !ok {
			return false
		}
		switch typ := literal.Type.(type) {
		case *ast.Ident:
			return strings.HasSuffix(typ.Name, "Error")
		case *ast.SelectorExpr:
			return strings.HasSuffix(typ.Sel.Name, "Error")
		default:
			return false
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if isStdCall(node, "errors", "New") {
				add(node.Pos(), "禁止 errors.New")
			}
			if isStdCall(node, "fmt", "Errorf") {
				add(node.Pos(), "禁止 fmt.Errorf")
			}
		case *ast.CallExpr:
			id, ok := node.Fun.(*ast.Ident)
			if !ok || id.Name != "panic" || len(node.Args) != 1 {
				break
			}
			switch arg := node.Args[0].(type) {
			case *ast.BasicLit:
				if arg.Kind == token.STRING {
					add(node.Pos(), "禁止 panic(string)")
				} else {
					add(node.Pos(), "panic 值须为类型错误")
				}
			case *ast.CallExpr:
				if isStdCall(arg.Fun, "fmt", "Sprintf") {
					add(node.Pos(), "禁止 panic(fmt.Sprintf)")
				} else {
					add(node.Pos(), "panic 值须为类型错误")
				}
			default:
				if !isTypedPanicArg(arg) {
					add(node.Pos(), "panic 值须为类型错误")
				}
			}
		}
		return true
	})
	return violations, nil
}

// scanErrorPolicy 检查仓库内已跟踪和未跟踪的生产 Go 文件，含独立子模块。
// 输入 root 是 Git 仓库根目录。
// 返回全部违规；列文件、读文件或解析失败时返回错误，不能静默放行。
func scanErrorPolicy(root string) ([]string, error) {
	listed, err := gitRaw(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.go")
	if err != nil {
		return nil, &os.PathError{Op: "git ls-files", Path: root, Err: err}
	}
	var violations []string
	seen := make(map[string]struct{})
	for _, rel := range strings.Split(listed, "\x00") {
		if rel == "" || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if _, exists := seen[rel]; exists {
			continue
		}
		seen[rel] = struct{}{}
		src, readErr := os.ReadFile(filepath.Join(root, rel))
		if os.IsNotExist(readErr) {
			continue // 工作区中已删除的跟踪文件
		}
		if readErr != nil {
			return nil, &os.PathError{Op: "read", Path: rel, Err: readErr}
		}
		found, parseErr := findDirectErrorConstructors(rel, src)
		if parseErr != nil {
			return nil, parseErr
		}
		violations = append(violations, found...)
	}
	return violations, nil
}
