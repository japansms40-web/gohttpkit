package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// kitErrorsPath 是 gohttpkit 结构化错误包的导入路径；Op / Kind 规则只认这个包的 Error 与 NewKind。
const kitErrorsPath = "github.com/japansms40-web/gohttpkit/errors"

// kindNameRe 约束 Kind 名称：<包>.<分类>，全小写 snake_case、点分，至少两段。
var kindNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// findDirectErrorConstructors 检查生产 Go 源码的直接错误构造；测试夹具不受此规则限制。
// 除标准库构造与字符串 panic 外，还按 CODE_STANDARDS §5.1 检查 gohttpkit errors 包的用法：
// Error 字面量的 Op 须引用具名 const、NewKind 只能在包级 var 声明、Kind 名须小写点分字面量。
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
	c := &errorPolicyChecker{
		filename:       filename,
		fset:           fset,
		imports:        make(map[string]string),
		kitAliases:     make(map[string]bool),
		allowedNewKind: make(map[*ast.CallExpr]bool),
	}
	c.collectImports(f)
	c.collectAllowedNewKind(f)
	ast.Inspect(f, c.visit)
	return c.violations, nil
}

// errorPolicyChecker 是单个文件的一次检查；只在 findDirectErrorConstructors 内顺序使用，不共享、不并发。
type errorPolicyChecker struct {
	filename string
	fset     *token.FileSet
	// imports 是标准库 errors / fmt 的本地名 → 导入路径。
	imports map[string]string
	// kitAliases 是 gohttpkit errors 包在本文件里的本地名。
	kitAliases map[string]bool
	// allowedNewKind 是直接作为包级 var 值的 NewKind 调用。
	allowedNewKind map[*ast.CallExpr]bool
	violations     []string
}

func (c *errorPolicyChecker) add(pos token.Pos, rule string) {
	c.violations = append(c.violations, fmt.Sprintf("%s:%d: %s", c.filename, c.fset.Position(pos).Line, rule))
}

// collectImports 记录 gohttpkit errors 包的别名与标准库 errors / fmt 的本地名；点导入标准库直接记违规。
func (c *errorPolicyChecker) collectImports(f *ast.File) {
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if path == kitErrorsPath {
			alias := "errors"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			c.kitAliases[alias] = true
			continue
		}
		if path != "errors" && path != "fmt" {
			continue
		}
		name := path
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			c.add(imp.Pos(), "禁止点导入标准库 errors/fmt")
			continue
		}
		c.imports[name] = path
	}
}

// collectAllowedNewKind 记录包级 var 声明里直接作为值的 NewKind 调用；嵌套在切片、函数体里的都不算。
func (c *errorPolicyChecker) collectAllowedNewKind(f *ast.File) {
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				if call, ok := v.(*ast.CallExpr); ok && c.isKitSel(call.Fun, "NewKind") {
					c.allowedNewKind[call] = true
				}
			}
		}
	}
}

// visit 是 ast.Inspect 的回调：按节点类型分派到各条规则，始终继续向下遍历。
func (c *errorPolicyChecker) visit(n ast.Node) bool {
	switch node := n.(type) {
	case *ast.CompositeLit:
		if c.isKitSel(node.Type, "Error") {
			c.checkErrorLiteral(node)
		}
	case *ast.SelectorExpr:
		if c.isStdCall(node, "errors", "New") {
			c.add(node.Pos(), "禁止 errors.New")
		}
		if c.isStdCall(node, "fmt", "Errorf") {
			c.add(node.Pos(), "禁止 fmt.Errorf")
		}
	case *ast.CallExpr:
		if c.isKitSel(node.Fun, "NewKind") {
			c.checkNewKind(node)
		} else {
			c.checkPanic(node)
		}
	}
	return true
}

func (c *errorPolicyChecker) isStdCall(expr ast.Expr, path, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && c.imports[id.Name] == path
}

func (c *errorPolicyChecker) isKitSel(expr ast.Expr, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && c.kitAliases[id.Name]
}

func (c *errorPolicyChecker) checkNewKind(call *ast.CallExpr) {
	if !c.allowedNewKind[call] {
		c.add(call.Pos(), "NewKind 只能在包级 var 声明")
	}
	if len(call.Args) == 1 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if name, err := strconv.Unquote(lit.Value); err == nil && kindNameRe.MatchString(name) {
				return
			}
		}
	}
	c.add(call.Pos(), "Kind 名须为 <包>.<分类> 小写点分字符串字面量")
}

func (c *errorPolicyChecker) checkErrorLiteral(lit *ast.CompositeLit) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "Op" {
			continue
		}
		switch kv.Value.(type) {
		case *ast.Ident, *ast.SelectorExpr:
		default:
			c.add(kv.Value.Pos(), "errors.Error 的 Op 须引用具名 const")
		}
	}
}

// checkPanic 检查单参数的 panic 调用；其它调用直接忽略。
func (c *errorPolicyChecker) checkPanic(call *ast.CallExpr) {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "panic" || len(call.Args) != 1 {
		return
	}
	switch arg := call.Args[0].(type) {
	case *ast.BasicLit:
		if arg.Kind == token.STRING {
			c.add(call.Pos(), "禁止 panic(string)")
			return
		}
	case *ast.CallExpr:
		if c.isStdCall(arg.Fun, "fmt", "Sprintf") {
			c.add(call.Pos(), "禁止 panic(fmt.Sprintf)")
			return
		}
	default:
		if isTypedPanicArg(arg) {
			return
		}
	}
	c.add(call.Pos(), "panic 值须为类型错误")
}

// isTypedPanicArg 判断 panic 参数是否为已有错误变量 err 或直接构造的 &XxxError{}；其它表达式可能产生字符串值。
// 这是语法门禁，err 的实际类型仍需调用点审查。
func isTypedPanicArg(expr ast.Expr) bool {
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
	for rel := range strings.SplitSeq(listed, "\x00") {
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
