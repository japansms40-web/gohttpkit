package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// style_policy.go —— check-style：可机器判定、历史上反复整改过的代码规则。
//   lock-defer        x.Lock() / x.RLock() 的下一条语句必须是 defer x.Unlock() / defer x.RUnlock()（CODE_STANDARDS §4）
//   event-decl        logger.NewEvent 只能出现在 events.go 的包级 var（§6）
//   panic-placement   panic 只能在 Must* / must* / init / package main 的 main，或带 //agentguard:allow-panic <理由> 的函数里
//   root-ctx          生产代码不调用 context.Background() / context.TODO()（package main 与 style.root_ctx_allow 前缀放行）
//   helper-placement  声明了方法的文件里不放包级未导出纯辅助函数（style.helper_placement 开启，§2）

const (
	ruleLockDefer       = "lock-defer"
	ruleEventDecl       = "event-decl"
	rulePanicPlacement  = "panic-placement"
	ruleRootCtx         = "root-ctx"
	ruleHelperPlacement = "helper-placement"
)

// kitLoggerPath 是 gohttpkit 日志门面的导入路径；event-decl 只认这个包的 NewEvent。
const kitLoggerPath = "github.com/japansms40-web/gohttpkit/logger"

// unlockOf 是加锁方法到对应解锁方法的映射。
var unlockOf = map[string]string{"Lock": "Unlock", "RLock": "RUnlock"}

// allowPanicRe 是放行 panic 的函数级指令，必须带非空理由。
var allowPanicRe = regexp.MustCompile(`^//agentguard:allow-panic[ \t]+\S`)

// styleOptions 是 check-style 的仓库级开关，来自 HEAD 里的 .agentguard.yml。
type styleOptions struct {
	helperPlacement       bool
	helperPlacementExempt []string
	rootCtxAllow          []string
}

// findStyleViolations 检查单个生产 Go 文件的代码规则；测试文件与生成文件不检查。
// 输入 filename：仓库相对路径（定位行号、判断 events.go / common.go / 放行前缀）；src：源码；opts：仓库开关。
// 返回：「路径:行: 规则：说明」形式的违规，按规则分组、组内按源码顺序；源码解析失败时返回错误，调用方必须阻断。
func findStyleViolations(filename string, src []byte, opts styleOptions) ([]string, error) {
	if strings.HasSuffix(filename, "_test.go") {
		return nil, nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if ast.IsGenerated(f) {
		return nil, nil
	}
	c := &styleChecker{filename: filename, fset: fset}
	c.checkLockDefer(f)
	c.checkEventDecl(f)
	c.checkPanicPlacement(f)
	c.checkRootCtx(f, opts.rootCtxAllow)
	if opts.helperPlacement {
		c.checkHelperPlacement(f, opts.helperPlacementExempt)
	}
	return c.violations, nil
}

// styleChecker 是单个文件的一次检查；只在 findStyleViolations 内顺序使用，不共享、不并发。
type styleChecker struct {
	filename   string
	fset       *token.FileSet
	violations []string
}

// add 记录一条违规；行号取真实源码行，不受 //line 指令影响。
// 输入 pos：违规位置；rule：规则名；msg：说明。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) add(pos token.Pos, rule, msg string) {
	c.violations = append(c.violations, c.filename+":"+strconv.Itoa(c.fset.PositionFor(pos, false).Line)+": "+rule+"："+msg)
}

// checkLockDefer 遍历全部语句列表（代码块、switch case、select 分支），检查加锁后紧跟 defer 解锁。
// 需要提前解锁的临界区应抽成函数或闭包，而不是手写 Unlock；TryLock 不在此列。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) checkLockDefer(f *ast.File) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			c.lockDeferInList(node.List)
		case *ast.CaseClause:
			c.lockDeferInList(node.Body)
		case *ast.CommClause:
			c.lockDeferInList(node.Body)
		}
		return true
	})
}

// lockDeferInList 检查一个语句列表里每个加锁语句的下一条是否为对应的 defer 解锁。
// 输入 list：语句列表。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) lockDeferInList(list []ast.Stmt) {
	for i, st := range list {
		recv, lock, ok := lockCall(st)
		if !ok {
			continue
		}
		unlock := unlockOf[lock]
		if i+1 < len(list) && isDeferCall(list[i+1], recv, unlock) {
			continue
		}
		c.add(st.Pos(), ruleLockDefer, recv+"."+lock+"() 的下一条语句须是 defer "+recv+"."+unlock+"()（需要提前解锁时把临界区抽成函数或闭包）")
	}
}

// lockCall 判断 st 是否为无参的 x.Lock() / x.RLock() 表达式语句。
// 返回：接收者表达式文本、加锁方法名与是否命中。
func lockCall(st ast.Stmt) (recv, lock string, ok bool) {
	es, isExpr := st.(*ast.ExprStmt)
	if !isExpr {
		return "", "", false
	}
	call, isCall := es.X.(*ast.CallExpr)
	if !isCall || len(call.Args) != 0 {
		return "", "", false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || unlockOf[sel.Sel.Name] == "" {
		return "", "", false
	}
	return types.ExprString(sel.X), sel.Sel.Name, true
}

// isDeferCall 判断 st 是否为 defer <recv>.<method>()。
// 输入 st：语句；recv：接收者表达式文本；method：解锁方法名。
// 返回：是则 true。
func isDeferCall(st ast.Stmt, recv, method string) bool {
	d, ok := st.(*ast.DeferStmt)
	if !ok || len(d.Call.Args) != 0 {
		return false
	}
	sel, ok := d.Call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == method && types.ExprString(sel.X) == recv
}

// checkEventDecl 检查 logger.NewEvent 只作为 events.go 包级 var 的值出现；点导入时按裸 NewEvent 匹配。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) checkEventDecl(f *ast.File) {
	alias := importAlias(f, kitLoggerPath, "logger")
	if alias == "" {
		return
	}
	allowed := make(map[*ast.CallExpr]bool)
	if path.Base(c.filename) == "events.go" {
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
					if call, ok := v.(*ast.CallExpr); ok {
						allowed[call] = true
					}
				}
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && isPkgSel(call.Fun, alias, "NewEvent") && !allowed[call] {
			c.add(call.Pos(), ruleEventDecl, "logger.NewEvent 只能写在本包 events.go 的包级 var（事件名是下游过滤契约，见 CODE_STANDARDS 日志一节）")
		}
		return true
	})
}

// importAlias 返回 importPath 在文件里的本地名。
// 输入 def：未起别名时的默认包名。
// 返回：本地名；点导入返回 "."；未导入或空白导入返回空串。
func importAlias(f *ast.File, importPath, def string) string {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		if imp.Name == nil {
			return def
		}
		if imp.Name.Name == "_" {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

// isPkgSel 判断 expr 是否为 <alias>.<name>；alias 为 "." 时（点导入）匹配裸标识符 <name>。
// 输入 expr：被调表达式；alias：包的本地名；name：成员名。
// 返回：命中则 true。
func isPkgSel(expr ast.Expr, alias, name string) bool {
	if alias == "." {
		id, ok := expr.(*ast.Ident)
		return ok && id.Name == name
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == alias
}

// checkPanicPlacement 检查 panic 只出现在允许的函数里（CODE_STANDARDS「资源与生命周期」：请求路径不 panic）。
// 包级 var 初始化里的 panic 一律不放行：需要启动期 fail-fast 时写成 Must* 函数。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) checkPanicPlacement(f *ast.File) {
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && panicAllowed(f, fd) {
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
				c.add(call.Pos(), rulePanicPlacement, "panic 只能在 Must*/must* 函数、init、package main 的 main，或函数注释带 //agentguard:allow-panic <理由> 的启动期注册函数里")
			}
			return true
		})
	}
}

// panicAllowed 判断函数 fd 内是否允许 panic。
// 返回：init、package main 的 main、Must*/must* 前缀函数（含方法）、doc 带 allow-panic 指令且写了理由的函数为 true。
func panicAllowed(f *ast.File, fd *ast.FuncDecl) bool {
	name := fd.Name.Name
	switch {
	case fd.Recv == nil && name == "init":
		return true
	case fd.Recv == nil && name == "main" && f.Name.Name == "main":
		return true
	case hasMustPrefix(name):
		return true
	}
	if fd.Doc != nil {
		for _, cm := range fd.Doc.List {
			if allowPanicRe.MatchString(cm.Text) {
				return true
			}
		}
	}
	return false
}

// hasMustPrefix 判断函数名是否以 Must / must 开头且其后为空或大写字母。
// 例：MustGet、mustFileWriter → true；mustard → false。
// 返回：满足前缀约定则 true。
func hasMustPrefix(name string) bool {
	rest, ok := strings.CutPrefix(name, "Must")
	if !ok {
		rest, ok = strings.CutPrefix(name, "must")
	}
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return unicode.IsUpper(r)
}

// checkRootCtx 检查生产代码不硬造根 ctx：context.Background() / context.TODO()（CODE_STANDARDS context 一节）。
// 输入 allow：放行的目录（.agentguard.yml style.root_ctx_allow，规范化后按目录边界前缀匹配）。package main 整体放行：CLI 与示例没有上游 ctx。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) checkRootCtx(f *ast.File, allow []string) {
	if f.Name.Name == "main" {
		return
	}
	for _, p := range allow {
		// 先 Clean（去掉 ./ 与末尾 /）再补 /，按目录边界匹配，避免 log 误放行 login/。
		if p = path.Clean(p); p != "." && strings.HasPrefix(c.filename, p+"/") {
			return
		}
	}
	alias := importAlias(f, "context", "context")
	if alias == "" {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && (isPkgSel(call.Fun, alias, "Background") || isPkgSel(call.Fun, alias, "TODO")) {
			c.add(call.Pos(), ruleRootCtx, "生产代码不硬造根 ctx：沿调用链透传 ctx；拿不到 ctx 的日志用 logger.Named，后台 goroutine 用 context.WithoutCancel(ctx)")
		}
		return true
	})
}

// checkHelperPlacement 检查声明了方法的文件里没有包级未导出纯辅助函数（CODE_STANDARDS §2「辅助函数归位」）。
// 输入 exempt：放行的文件完整路径（.agentguard.yml style.helper_placement_exempt），按完整路径精确匹配。
// 例外：common.go 自身；exempt 列出的文件（先 path.Clean）；类型构造器 newXxx；首参为 context.Context 的端点主实现；init、main 与 _。
// 返回：无，违规追加到 c.violations。
func (c *styleChecker) checkHelperPlacement(f *ast.File, exempt []string) {
	if path.Base(c.filename) == "common.go" || slices.ContainsFunc(exempt, func(e string) bool { return path.Clean(e) == c.filename }) {
		return
	}
	ctxAlias := importAlias(f, "context", "context")
	hasMethod := false
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil {
			hasMethod = true
			break
		}
	}
	if !hasMethod {
		return
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || !isHelperFunc(fd, ctxAlias) {
			continue
		}
		c.add(fd.Pos(), ruleHelperPlacement, "纯辅助函数 "+fd.Name.Name+" 须移到本包 common.go（本文件声明了方法；用例同步移到 common_test.go）")
	}
}

// isHelperFunc 判断包级函数 fd 是否属于须归位的纯辅助函数。
// 输入 fd：包级函数声明；ctxAlias：context 包在本文件的本地名（点导入为 "."，未导入为空串）。
// 返回：属于须归位的辅助函数则 true。
func isHelperFunc(fd *ast.FuncDecl, ctxAlias string) bool {
	name := fd.Name.Name
	if ast.IsExported(name) || name == "init" || name == "main" || name == "_" || isConstructorName(name) {
		return false
	}
	if params := fd.Type.Params.List; len(params) > 0 {
		if ctxAlias != "" && isPkgSel(params[0].Type, ctxAlias, "Context") {
			return false
		}
	}
	return true
}

// isConstructorName 判断是否为类型构造器名 newXxx（new 后紧跟大写字母）。
// 例：newClient → true；newline → false。
// 返回：是构造器名则 true。
func isConstructorName(name string) bool {
	rest, ok := strings.CutPrefix(name, "new")
	if !ok || rest == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return unicode.IsUpper(r)
}

// scanStyle 对仓库内已跟踪与未跟踪（不含忽略）的生产 Go 文件跑 check-style，含独立子模块，跳过 testdata/。
// 输入 root：Git 仓库根；开关取 HEAD 里的 .agentguard.yml（见 loadStaticConfig）。
// 返回：排序后的全部违规；读配置、列文件、读文件或解析失败时返回错误，不能静默放行。
func scanStyle(root string) ([]string, error) {
	cfg, err := loadStaticConfig(root)
	if err != nil {
		return nil, err
	}
	opts := styleOptions{
		helperPlacement:       cfg.Style.HelperPlacement,
		helperPlacementExempt: cfg.Style.HelperPlacementExempt,
		rootCtxAllow:          cfg.Style.RootCtxAllow,
	}
	files, err := presentGoFiles(root)
	if err != nil {
		return nil, err
	}
	var violations []string
	for rel := range files {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		src, readErr := os.ReadFile(filepath.Join(root, rel))
		if readErr != nil {
			return nil, &os.PathError{Op: "read", Path: rel, Err: readErr}
		}
		found, parseErr := findStyleViolations(rel, src, opts)
		if parseErr != nil {
			return nil, parseErr
		}
		violations = append(violations, found...)
	}
	slices.SortFunc(violations, compareViolation)
	return violations, nil
}

// violationLocRe 从「路径:行: 规则：说明」里取路径与行号。
var violationLocRe = regexp.MustCompile(`^(.*?):(\d+): `)

// compareViolation 按路径、行号数值、原字符串比较两条违规。
// 输入 a、b：findStyleViolations 产出的违规文本。
// 返回：a 在前为负，相等为 0，a 在后为正。
func compareViolation(a, b string) int {
	pa, la := splitViolation(a)
	pb, lb := splitViolation(b)
	if c := strings.Compare(pa, pb); c != 0 {
		return c
	}
	if la != lb {
		return la - lb
	}
	return strings.Compare(a, b)
}

// splitViolation 解析违规文本的路径与行号。
// 返回：路径与行号；格式不符时返回整串与 0。
func splitViolation(v string) (string, int) {
	m := violationLocRe.FindStringSubmatch(v)
	if m == nil {
		return v, 0
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return v, 0
	}
	return m[1], n
}
