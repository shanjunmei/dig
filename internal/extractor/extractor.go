package extractor

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/alias"
	"github.com/shanjunmei/dig/pkg/functional"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

const (
	diPkgPath            = model.DiPkgPath
	closurePrefixInvoke  = "dig_invoke_"
	closurePrefixProvide = "dig_provider_"
)

type Extractor struct {
	pkgMap            map[string]*packages.Package
	mainPkgPath       string
	items             []extractedItem
	globalProviderMap map[string]int
	pkgAliasMap       map[string]string
	pkgNameMap        map[string]string
	importAliasMap    map[string]string
	typeStrCache      map[types.Type]string
	aliasStrategy     alias.AliasStrategy
	invokeIndex       int
	provideIndex      int
	moduleRoot        string
	cfg               *config.Config
}

// ---------- 新模型 ----------
type ExtractedArg struct {
	model.Arg
	Type       types.Type
	TypeString string
}

type extractedItem struct {
	FuncName string
	RetType  string
	IsInvoke bool
	IsSupply bool
	Expr     ast.Expr
	Pkg      *packages.Package
	PkgAlias string
	HasError bool
	UsedPkgs []string

	IsClosure       bool
	ClosureLit      *ast.FuncLit
	FreeVars        []*ast.Ident
	FreeTypes       []types.Type
	FreeTypeStrings []string

	Params        []ExtractedArg // 合并后的参数列表（闭包参数 + 自由变量）
	ClosureParams []ExtractedArg // 闭包自身的原始参数

	GenericArgsStr string

	SourceComment string

	Position string

	// ---------- 新增字段 ----------
	InstanceName string // 实例名称（命名返回值或 Supply 表达式名称）
}

// findModuleRoot 向上查找 go.mod 所在目录
func findModuleRoot(startDir string) string {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
func (e *Extractor) relPath(absPath string) string {
	if e.moduleRoot == "" {
		return filepath.Base(absPath)
	}
	rel, err := filepath.Rel(e.moduleRoot, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	return filepath.ToSlash(rel)
}

// NewExtractor 创建提取器
func NewExtractor(cfg *config.Config, pkgMap map[string]*packages.Package, mainPkgPath string, strategy alias.AliasStrategy, startDir string) *Extractor {
	rootDir := findModuleRoot(startDir)
	e := &Extractor{
		cfg:               cfg,
		pkgMap:            pkgMap,
		mainPkgPath:       mainPkgPath,
		items:             []extractedItem{},
		globalProviderMap: make(map[string]int),
		pkgAliasMap:       make(map[string]string),
		pkgNameMap:        make(map[string]string),
		importAliasMap:    make(map[string]string),
		typeStrCache:      make(map[types.Type]string),
		aliasStrategy:     strategy,
		moduleRoot:        rootDir,
	}
	e.loadImportAliases()
	return e
}
func (e *Extractor) ConditionalDebugf(pred func() bool, tpl string, args ...any) string {
	if !e.cfg.Debug || !pred() {
		return ""
	}
	return fmt.Sprintf(tpl, args...)
}

// ---------- 辅助构造函数 ----------
func newExtractedArg(name string, typ types.Type, typeStr string, isConst bool, constVal string, isCtx bool) ExtractedArg {
	return ExtractedArg{
		Arg: model.Arg{
			Name:       name,
			IsConst:    isConst,
			ConstValue: constVal,
			IsContext:  isCtx,
		},
		Type:       typ,
		TypeString: typeStr,
	}
}

// addPkgToUsed 将类型所在的非主包添加到 usedPkgs 中
func (e *Extractor) addPkgToUsed(typ types.Type, usedPkgs map[string]bool) {
	switch t := typ.(type) {
	case *types.Map:
		// 分别处理键和值
		if pkg := e.typePkg(t.Key()); pkg != nil && pkg.Path() != e.mainPkgPath {
			usedPkgs[pkg.Path()] = true
		}
		if pkg := e.typePkg(t.Elem()); pkg != nil && pkg.Path() != e.mainPkgPath {
			usedPkgs[pkg.Path()] = true
		}
	default:
		if pkg := e.typePkg(t); pkg != nil && pkg.Path() != e.mainPkgPath {
			usedPkgs[pkg.Path()] = true
		}
	}
}

// replaceTypeNames 使用类型别名替换 body 字符串中的类型名（正则匹配标识符边界）
// 注意：不匹配已经带有包前缀的类型名（如 approval.Adapter 中的 Adapter）
func replaceTypeNames(bodyStr string, typeNameMap map[string]string) string {
	for name, replacement := range typeNameMap {
		// 匹配独立的标识符：前面是行首、空白、左括号、逗号、等号等，后面是行尾、空白、右括号、逗号、点等
		re := regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(name) + `([^\w]|$)`)
		bodyStr = re.ReplaceAllString(bodyStr, `${1}`+replacement+`${2}`)
	}
	return bodyStr
}

// buildExtractedParams 从签名构建参数列表（保留参数名）
// 注意：现在会保留参数名，用于命名依赖匹配
func (e *Extractor) buildExtractedParams(sig *types.Signature) []ExtractedArg {
	n := sig.Params().Len()
	params := make([]ExtractedArg, n)
	for i := range n {
		param := sig.Params().At(i)
		typ := param.Type()
		typeStr := e.getTypeFullName(typ)
		isCtx := isContextType(typ)
		// 保留参数名
		params[i] = newExtractedArg(param.Name(), typ, typeStr, false, "", isCtx)
	}
	return params
}

// extractGenericArgStr 从带泛型索引的 expr 取出 [T1,T2] 字符串，清洗包路径
func (e *Extractor) extractGenericArgStr(expr ast.Expr, curPkg *packages.Package) (string, error) {
	_, indexNode := stripGenericIndexes(expr)
	if indexNode == nil {
		return "", nil
	}

	var buf bytes.Buffer
	switch idx := indexNode.(type) {
	case *ast.IndexExpr:
		if err := printer.Fprint(&buf, curPkg.Fset, idx.Index); err != nil {
			return "", err
		}
		return "[" + e.replacePkgPathWithAlias(buf.String()) + "]", nil
	case *ast.IndexListExpr:
		var parts []string
		for _, item := range idx.Indices {
			var subBuf bytes.Buffer
			if err := printer.Fprint(&subBuf, curPkg.Fset, item); err != nil {
				return "", err
			}
			parts = append(parts, e.replacePkgPathWithAlias(subBuf.String()))
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	default:
		return "", nil
	}
}

func (e *Extractor) extractOptions(expr ast.Expr, curPkg, realPkg *packages.Package) error {
	expr = ast.Unparen(expr)
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		pos := curPkg.Fset.Position(expr.Pos())
		return fmt.Errorf("at %s: invalid option expression (expected a call expression, got %T)", pos, expr)
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		obj := curPkg.TypesInfo.ObjectOf(sel.Sel)
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == diPkgPath {
			switch obj.Name() {
			case "Provide":
				return e.processArgs(call.Args, realPkg, e.handleProvide)
			case "Invoke":
				return e.processArgs(call.Args, realPkg, e.handleInvoke)
			case "Supply":
				return e.processArgs(call.Args, realPkg, e.handleSupply)
			case "Module":
				return e.processArgs(call.Args, curPkg, func(arg ast.Expr, _ *packages.Package) error {
					return e.extractOptions(arg, curPkg, realPkg)
				})
			}
		}
	}
	return e.extractOptionsFromFuncCall(call, curPkg)
}

// stripGenericIndexes 剥离泛型索引表达式，返回最底层的表达式和最后一个索引节点（如果有）
func stripGenericIndexes(expr ast.Expr) (base ast.Expr, indexNode ast.Node) {
	for {
		switch n := expr.(type) {
		case *ast.IndexExpr:
			indexNode = n
			expr = n.X
		case *ast.IndexListExpr:
			indexNode = n
			expr = n.X
		default:
			return expr, indexNode
		}
	}
}

// isErrorType reports whether typ is the built-in error type.
func isErrorType(typ types.Type) bool {
	return types.Identical(typ, types.Universe.Lookup("error").Type())
}

// ---------- resolveFunctionObject 保持原样 ----------
func resolveFunctionObject(call *ast.CallExpr, curPkg *packages.Package) types.Object {
	base, _ := stripGenericIndexes(call.Fun)
	switch fun := base.(type) {
	case *ast.Ident:
		return curPkg.TypesInfo.ObjectOf(fun)
	case *ast.SelectorExpr:
		return curPkg.TypesInfo.ObjectOf(fun.Sel)
	default:
		return nil
	}
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r := rune(name[0])
	return r >= 'A' && r <= 'Z'
}

func checkExportedVisibility(obj types.Object, curPkg *types.Package) error {
	defPkg := obj.Pkg()
	if defPkg == nil || curPkg == defPkg {
		return nil
	}
	if !isExported(obj.Name()) {
		return fmt.Errorf("cross-package unexported: %s (pkg: %s)", obj.Name(), defPkg.Path())
	}
	return nil
}

func (e *Extractor) typeQualifier(p *types.Package) string {
	return p.Path()
}

func (e *Extractor) getTypeFullName(typ types.Type) string {
	if s, ok := e.typeStrCache[typ]; ok {
		return s
	}
	s := types.TypeString(typ, e.typeQualifier)
	e.typeStrCache[typ] = s
	return s
}
func (e *Extractor) PackageNameMap() map[string]string {
	return e.pkgNameMap
}

func (e *Extractor) ImportAliasMap() map[string]string {
	return e.importAliasMap
}

func (e *Extractor) collectPkgAlias(pkg *packages.Package) string {
	if pkg != nil {
		e.pkgNameMap[pkg.PkgPath] = pkg.Name
	}
	pp := pkg.PkgPath
	if pp == "" {
		return ""
	}
	if alias, ok := e.importAliasMap[pp]; ok {
		if pp != e.mainPkgPath {
			e.pkgAliasMap[pp] = alias
			return alias
		}
		return ""
	}
	if alias, ok := e.pkgAliasMap[pp]; ok {
		if pp == e.mainPkgPath {
			return ""
		}
		return alias
	}
	existing := make(map[string]bool)
	for _, a := range e.pkgAliasMap {
		existing[a] = true
	}
	for _, a := range e.importAliasMap {
		existing[a] = true
	}
	alias := e.aliasStrategy.GenerateAlias(pp, existing)
	if pp != e.mainPkgPath {
		e.pkgAliasMap[pp] = alias
	}
	if pp == e.mainPkgPath {
		return ""
	}
	return alias
}

// collectUsedPkgsFromType 递归提取类型中引用的非主包路径
func (e *Extractor) collectUsedPkgsFromType(typ types.Type) []string {
	var pkgs []string
	seen := make(map[string]bool)
	var walk func(t types.Type)
	walk = func(t types.Type) {
		switch t := t.(type) {
		case *types.Named:
			if pkg := t.Obj().Pkg(); pkg != nil && pkg.Path() != e.mainPkgPath {
				if !seen[pkg.Path()] {
					seen[pkg.Path()] = true
					pkgs = append(pkgs, pkg.Path())
				}
			}
			if params := t.TypeParams(); params != nil {
				for tparam := range params.TypeParams() {
					walk(tparam)
				}
			}
		case *types.Pointer, *types.Slice, *types.Array, *types.Chan:
			walk(t.(interface{ Elem() types.Type }).Elem())
		case *types.Map:
			walk(t.Key())
			walk(t.Elem())
		case *types.Struct:
			for field := range t.Fields() {
				walk(field.Type())
			}
		case *types.Interface:
			for method := range t.Methods() {
				walk(method.Type())
			}
		}
	}
	walk(typ)
	return pkgs
}

// populateUsedPkgs 为所有非闭包 item 填充 UsedPkgs
func (e *Extractor) populateUsedPkgs() {
	for i := range e.items {
		it := &e.items[i]
		if it.IsClosure {
			continue
		}
		if len(it.UsedPkgs) > 0 {
			continue
		}
		usedMap := make(map[string]bool)

		// 从表达式中收集（函数名/值可能带包前缀）
		if it.Expr != nil {
			// 语法层面收集（如选择器）
			for _, p := range e.collectUsedPkgsFromExpr(it.Expr, it.Pkg.TypesInfo) {
				usedMap[p] = true
			}
			// 类型层面收集（如标识符）
			typ := it.Pkg.TypesInfo.TypeOf(it.Expr)
			if typ != nil {
				for _, p := range e.collectUsedPkgsFromType(typ) {
					usedMap[p] = true
				}
			}
		}

		// 从参数类型收集
		for _, arg := range it.Params {
			if arg.Type != nil {
				for _, p := range e.collectUsedPkgsFromType(arg.Type) {
					usedMap[p] = true
				}
			}
		}

		// 转为切片
		used := make([]string, 0, len(usedMap))
		for p := range usedMap {
			used = append(used, p)
		}
		it.UsedPkgs = used
	}
}

// ---------- 新增辅助函数：提取实例名称 ----------

// extractNamedReturn 提取函数签名的命名返回值名称
// 例如 func() (mainDB *sql.DB) 返回 "mainDB"
func (e *Extractor) extractNamedReturn(sig *types.Signature) string {
	if sig.Results().Len() == 0 {
		return ""
	}
	first := sig.Results().At(0)
	if first == nil {
		return ""
	}
	return first.Name()
}

// extractSupplyName 从 Supply 表达式中提取实例名称
// 例如 dig.Supply(mainDB) 返回 "mainDB"
func (e *Extractor) extractSupplyName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if ident, ok := v.X.(*ast.Ident); ok {
			return ident.Name + "." + v.Sel.Name
		}
		return v.Sel.Name
	default:
		return ""
	}
}

// getRequiredInstanceName 获取依赖方需要的实例名称（从参数名）
func (e *Extractor) getRequiredInstanceName(arg ExtractedArg) string {
	if arg.Name != "" && arg.Name != "_" {
		return arg.Name
	}
	return ""
}

// ---------- handleSupply 修改 ----------
func (e *Extractor) handleSupply(expr ast.Expr, curPkg *packages.Package) error {
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := checkExportedVisibility(obj, curPkg.Types); err != nil {
			return err
		}
		if err := e.checkGenerationVisibility(obj); err != nil {
			return err
		}
	}
	alias := e.collectPkgAlias(curPkg)
	typ := curPkg.TypesInfo.TypeOf(expr)
	if typ == nil {
		return fmt.Errorf("resolve supply type failed")
	}
	retType := e.getTypeFullName(typ)
	usedPkgs := e.collectUsedPkgsFromExpr(expr, curPkg.TypesInfo)

	instanceName := e.extractSupplyName(expr)

	item := e.newExtractedItem("", curPkg, alias, false)
	item.IsSupply = true
	item.RetType = retType
	item.Expr = expr
	item.UsedPkgs = usedPkgs
	item.InstanceName = instanceName

	pos := curPkg.Fset.Position(expr.Pos())
	relPath := e.relPath(pos.Filename)
	sourceComment := e.ConditionalDebugf(func() bool { return true }, "// supply from %s at %s:%d", curPkg.PkgPath, relPath, pos.Line)
	item.SourceComment = sourceComment
	item.Position = fmt.Sprintf("%s:%d", relPath, pos.Line)

	// 构建键：默认键和命名键
	keyDefault := retType
	keyNamed := retType
	if instanceName != "" {
		keyNamed = retType + ":" + instanceName
	}
	// 检查命名键冲突
	if oldIdx, exists := e.globalProviderMap[keyNamed]; exists {
		oldDesc := e.describeItem(oldIdx)
		currentDesc := e.describeItemByIt(item)
		return fmt.Errorf("duplicate binding for %s with name %q:\n\tprevious: %s\n\tcurrent: %s",
			retType, instanceName, oldDesc, currentDesc)
	}
	if instanceName == "" {
		if oldIdx, exists := e.globalProviderMap[keyDefault]; exists {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("duplicate binding for %s (default):\n\tprevious: %s\n\tcurrent: %s",
				retType, oldDesc, currentDesc)
		}
	}

	idx := len(e.items)
	e.items = append(e.items, item)
	if instanceName != "" {
		e.globalProviderMap[keyNamed] = idx
	} else {
		e.globalProviderMap[keyDefault] = idx
	}
	return nil
}

func (e *Extractor) newExtractedItem(funcName string, pkg *packages.Package, alias string, hasErr bool) extractedItem {
	return extractedItem{
		FuncName: funcName,
		Pkg:      pkg,
		PkgAlias: alias,
		HasError: hasErr,
	}
}

func sigHasError(sig *types.Signature) bool {
	res := sig.Results()
	if res.Len() == 0 {
		return false
	}
	lastTyp := res.At(res.Len() - 1).Type()
	return isErrorType(lastTyp)
}

func (e *Extractor) extractClosureParams(funcLit *ast.FuncLit, curPkg *packages.Package) ([]string, []types.Type, []string) {
	var names []string
	var typesList []types.Type
	var typeStrs []string
	if funcLit.Type.Params != nil {
		total := 0
		for _, field := range funcLit.Type.Params.List {
			total += len(field.Names)
		}
		names = make([]string, 0, total)
		typesList = make([]types.Type, 0, total)
		typeStrs = make([]string, 0, total)
		for _, field := range funcLit.Type.Params.List {
			for _, name := range field.Names {
				names = append(names, name.Name)
				t := curPkg.TypesInfo.TypeOf(field.Type)
				typesList = append(typesList, t)
				typeStrs = append(typeStrs, e.getTypeFullName(t))
			}
		}
	}
	return names, typesList, typeStrs
}

func (e *Extractor) collectAssignDecls(assign *ast.AssignStmt, declSet map[string]bool) {
	for _, lhs := range assign.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Obj != nil && ident.Obj.Kind == ast.Var {
			declSet[ident.Name] = true
		}
	}
}

func (e *Extractor) collectParamDecls(funcLit *ast.FuncLit, declSet map[string]bool) {
	if funcLit.Type.Params != nil {
		for _, field := range funcLit.Type.Params.List {
			for _, name := range field.Names {
				declSet[name.Name] = true
			}
		}
	}
}

func (e *Extractor) collectGenDecls(decl *ast.DeclStmt, declSet map[string]bool) {
	genDecl, ok := decl.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range genDecl.Specs {
		valSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range valSpec.Names {
			declSet[name.Name] = true
		}
	}
}

func (e *Extractor) collectDeclarations(funcLit *ast.FuncLit) map[string]bool {
	declSet := make(map[string]bool)
	e.collectParamDecls(funcLit, declSet)
	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			e.collectAssignDecls(x, declSet)
		case *ast.DeclStmt:
			e.collectGenDecls(x, declSet)
		}
		return true
	})
	return declSet
}

func (e *Extractor) collectFreeVarsFromBody(body *ast.BlockStmt, curPkg *packages.Package, declSet map[string]bool) ([]*ast.Ident, []types.Type, []string, []bool, []string, error) {
	var freeVars []*ast.Ident
	var freeTypes []types.Type
	var freeTypeStrs []string
	var isConst []bool
	var litValues []string
	seen := make(map[string]bool)
	pkgScope := curPkg.Types.Scope()

	var err error
	ast.Inspect(body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := curPkg.TypesInfo.ObjectOf(ident)
		if obj == nil {
			return true
		}
		if _, isDecl := declSet[ident.Name]; isDecl {
			return true
		}

		switch o := obj.(type) {
		case *types.Var:
			if o.Parent() != pkgScope {
				if o.Pkg() == nil || o.Parent() == nil {
					return true
				}
				err = fmt.Errorf("cannot capture local variable %q defined in InitApp scope; pass it as a parameter to the function (preferred) or move it to package level", ident.Name)
				return false
			}
			if seen[ident.Name] {
				return true
			}
			seen[ident.Name] = true
			freeVars = append(freeVars, ident)
			freeTypes = append(freeTypes, obj.Type())
			freeTypeStrs = append(freeTypeStrs, e.getTypeFullName(obj.Type()))
			isConst = append(isConst, false)
			litValues = append(litValues, "")
			return true

		case *types.Const:
			if o.Parent() != pkgScope {
				if o.Pkg() == nil || o.Parent() == nil {
					return true
				}
				err = fmt.Errorf("cannot capture local constant %q defined in InitApp scope; pass it as a parameter to the function (preferred) or move it to package level", ident.Name)
				return false
			}
			return true

		default:
			return true
		}
	})

	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return freeVars, freeTypes, freeTypeStrs, isConst, litValues, nil
}

func (e *Extractor) checkFreeVarVisibility(vars []*ast.Ident, curPkg *packages.Package) error {
	for _, ident := range vars {
		obj := curPkg.TypesInfo.ObjectOf(ident)
		if obj != nil {
			if err := checkExportedVisibility(obj, curPkg.Types); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- isContextType 保持原样 ----------
func isContextType(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

func (e *Extractor) collectFreeVarsWithConst(funcLit *ast.FuncLit, curPkg *packages.Package) ([]*ast.Ident, []types.Type, []string, []bool, []string, error) {
	declSet := e.collectDeclarations(funcLit)
	freeVars, freeTypes, freeTypeStrs, isConst, litValues, err := e.collectFreeVarsFromBody(funcLit.Body, curPkg, declSet)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if err := e.checkFreeVarVisibility(freeVars, curPkg); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	for _, ident := range freeVars {
		obj := curPkg.TypesInfo.ObjectOf(ident)
		if obj != nil && isContextType(obj.Type()) {
			return nil, nil, nil, nil, nil, fmt.Errorf("cannot capture context variable %q as free variable; please pass context as a function parameter", ident.Name)
		}
	}
	return freeVars, freeTypes, freeTypeStrs, isConst, litValues, nil
}

func (e *Extractor) determineReturnType(funcLit *ast.FuncLit, sig *types.Signature, isInvoke bool, curPkg *packages.Package) (string, error) {
	if isInvoke {
		return "", nil
	}
	res := sig.Results()
	if res.Len() == 0 {
		return "", fmt.Errorf("anonymous provide function has no return")
	}
	if funcLit.Type.Results != nil && len(funcLit.Type.Results.List) > 0 {
		retExpr := funcLit.Type.Results.List[0].Type
		return e.getTypeFullName(curPkg.TypesInfo.TypeOf(retExpr)), nil
	}
	return e.getTypeFullName(res.At(0).Type()), nil
}

func (e *Extractor) generateFuncName(isInvoke bool) string {
	if isInvoke {
		e.invokeIndex++
		return fmt.Sprintf("%s%d", closurePrefixInvoke, e.invokeIndex)
	}
	e.provideIndex++
	return fmt.Sprintf("%s%d", closurePrefixProvide, e.provideIndex)
}

// ---------- handleFuncLit 修改 ----------
// ---------- 重构后的 handleFuncLit ----------
func (e *Extractor) handleFuncLit(funcLit *ast.FuncLit, curPkg *packages.Package, isInvoke bool) error {
	// 1. 验证签名
	sig, err := e.validateClosureSignature(funcLit, curPkg, isInvoke)
	if err != nil {
		return err
	}

	// 2. 检查闭包体内的私有方法调用
	if err := e.checkMethodVisibilityInClosure(funcLit.Body, curPkg); err != nil {
		return err
	}

	// 3. 构建参数列表和自由变量
	params, closureParams, freeVars, freeTypes, freeTypeStrs, err :=
		e.buildClosureArgumentLists(funcLit, curPkg)
	if err != nil {
		return err
	}

	// 4. 确定返回类型
	retType, err := e.determineReturnType(funcLit, sig, isInvoke, curPkg)
	if err != nil {
		return err
	}
	if retType != "" {
		if _, dup := e.globalProviderMap[retType]; dup {
			return fmt.Errorf("duplicate provide for type %q", retType)
		}
	}

	// 5. 构建 extractedItem
	funcName := e.generateFuncName(isInvoke)
	hasErr := sigHasError(sig)
	item := e.newExtractedItem(funcName, curPkg, e.collectPkgAlias(curPkg), hasErr)
	item.IsInvoke = isInvoke
	item.IsClosure = true
	item.ClosureLit = funcLit
	item.FreeVars = freeVars
	item.FreeTypes = freeTypes
	item.FreeTypeStrings = freeTypeStrs
	item.Params = params
	item.ClosureParams = closureParams
	if retType != "" {
		item.RetType = retType
	}
	item.InstanceName = e.extractNamedReturn(sig)

	// 设置位置信息
	pos := curPkg.Fset.Position(funcLit.Pos())
	relPath := e.relPath(pos.Filename)
	item.SourceComment = e.ConditionalDebugf(func() bool { return true }, "// closure defined at %s:%d", relPath, pos.Line)
	item.Position = fmt.Sprintf("%s:%d", relPath, pos.Line)

	// 6. 注册
	idx := len(e.items)
	e.items = append(e.items, item)
	if !isInvoke && retType != "" {
		if err := e.registerClosureProvider(item, idx); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 新增辅助函数 ----------

// validateClosureSignature 验证闭包的签名，返回 *types.Signature
func (e *Extractor) validateClosureSignature(funcLit *ast.FuncLit, curPkg *packages.Package, isInvoke bool) (*types.Signature, error) {
	typ := curPkg.TypesInfo.TypeOf(funcLit)
	sig, ok := typ.(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("func literal is not a function type")
	}
	if isInvoke {
		if err := validateInvokeSignature(sig, "anonymous function"); err != nil {
			return nil, err
		}
	}
	return sig, nil
}

// buildClosureArgumentLists 构建闭包的参数列表和自由变量
// 返回：完整参数列表（闭包参数+自由变量）、闭包自身参数列表、自由变量标识符、自由变量类型、自由变量类型字符串
func (e *Extractor) buildClosureArgumentLists(funcLit *ast.FuncLit, curPkg *packages.Package) (
	params []ExtractedArg,
	closureParams []ExtractedArg,
	freeVars []*ast.Ident,
	freeTypes []types.Type,
	freeTypeStrs []string,
	err error,
) {
	// 提取闭包参数
	paramNames, paramTypes, paramTypeStrs := e.extractClosureParams(funcLit, curPkg)

	// 收集自由变量
	freeVars, freeTypes, freeTypeStrs, freeIsConst, freeLitValues, err := e.collectFreeVarsWithConst(funcLit, curPkg)
	if err != nil {
		return
	}

	// 构建完整参数列表（闭包参数 + 自由变量）
	totalParams := len(paramNames) + len(freeVars)
	params = make([]ExtractedArg, totalParams)

	// 填充闭包参数
	for i := range paramNames {
		params[i] = newExtractedArg(
			paramNames[i],
			paramTypes[i],
			paramTypeStrs[i],
			false, "",
			isContextType(paramTypes[i]),
		)
	}

	// 填充自由变量
	for i := range freeVars {
		idx := len(paramNames) + i
		params[idx] = newExtractedArg(
			freeVars[i].Name,
			freeTypes[i],
			freeTypeStrs[i],
			freeIsConst[i],
			freeLitValues[i],
			false,
		)
	}

	// 构建闭包自身参数列表
	closureParams = make([]ExtractedArg, len(paramNames))
	for i := range paramNames {
		closureParams[i] = newExtractedArg(
			paramNames[i],
			paramTypes[i],
			paramTypeStrs[i],
			false, "",
			isContextType(paramTypes[i]),
		)
	}

	return
}

// registerClosureProvider 将闭包提供者注册到 globalProviderMap
func (e *Extractor) registerClosureProvider(item extractedItem, idx int) error {
	key := item.RetType
	if item.InstanceName != "" {
		key = item.RetType + ":" + item.InstanceName
	}
	if oldIdx, exists := e.globalProviderMap[key]; exists {
		if oldIdx != idx {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("duplicate binding for %s with name %q:\n\tprevious: %s\n\tcurrent: %s",
				item.RetType, item.InstanceName, oldDesc, currentDesc)
		}
	} else {
		e.globalProviderMap[key] = idx
	}
	return nil
}

func (e *Extractor) collectUsedPkgsFromExpr(expr ast.Expr, info *types.Info) []string {
	var pkgs []string
	seen := make(map[string]bool)
	ast.Inspect(expr, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.ObjectOf(ident)
		if obj == nil {
			return true
		}
		pkgName, ok := obj.(*types.PkgName)
		if !ok {
			return true
		}
		pkgPath := pkgName.Imported().Path()
		if pkgPath == "" || pkgPath == e.mainPkgPath {
			return true
		}
		if !seen[pkgPath] {
			seen[pkgPath] = true
			pkgs = append(pkgs, pkgPath)
		}
		return true
	})
	return pkgs
}

func getFuncMeta(expr ast.Expr, curPkg *packages.Package, pkgMap map[string]*packages.Package) (name string, sig *types.Signature, realPkg *packages.Package, err error) {
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj == nil {
		var buf strings.Builder
		_ = printer.Fprint(&buf, curPkg.Fset, expr)
		return "", nil, nil, fmt.Errorf("resolve object failed for expression: %s", buf.String())
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return "", nil, nil, fmt.Errorf("%s is not a function", obj.Name())
	}
	fnPkg := fn.Pkg()
	if fnPkg == nil {
		return "", nil, nil, fmt.Errorf("function %s has no package", fn.Name())
	}
	realPkg, ok = pkgMap[fnPkg.Path()]
	if !ok {
		return "", nil, nil, fmt.Errorf("package %s not found in pkgMap", fnPkg.Path())
	}
	instFuncType := curPkg.TypesInfo.TypeOf(expr)
	instSig, ok := instFuncType.(*types.Signature)
	if !ok {
		return "", nil, nil, fmt.Errorf("failed to get instantiated signature for %s", fn.Name())
	}

	return fn.Name(), instSig, realPkg, nil
}

func validateInvokeSignature(sig *types.Signature, funcName string) error {
	res := sig.Results()
	if res.Len() == 0 {
		return nil
	}
	if res.Len() == 1 {
		if !isErrorType(res.At(0).Type()) {
			return fmt.Errorf("invoke function %s: single return value must be error, got %s", funcName, res.At(0).Type().String())
		}
		return nil
	}
	return fmt.Errorf("invoke function %s has %d return values (only 0 or error allowed)", funcName, res.Len())
}

// ---------- handleInvoke 使用新模型 ----------
func (e *Extractor) handleInvoke(expr ast.Expr, curPkg *packages.Package) error {
	if funcLit, ok := expr.(*ast.FuncLit); ok {
		return e.handleFuncLit(funcLit, curPkg, true)
	}
	name, sig, realPkg, err := getFuncMeta(expr, curPkg, e.pkgMap)
	if err != nil {
		return err
	}
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := e.checkGenerationVisibility(obj); err != nil {
			return err
		}
	}
	if err := validateInvokeSignature(sig, name); err != nil {
		return err
	}
	genericStr, err := e.extractGenericArgStr(expr, curPkg)
	if err != nil {
		return err
	}
	alias := e.collectPkgAlias(realPkg)
	hasErr := sigHasError(sig)
	item := e.newExtractedItem(name, realPkg, alias, hasErr)
	item.IsInvoke = true
	item.Params = e.buildExtractedParams(sig) // 注意这里现在保留了参数名
	item.GenericArgsStr = genericStr
	e.items = append(e.items, item)
	return nil
}

// ---------- handleProvide 修改 ----------
func (e *Extractor) handleProvide(expr ast.Expr, curPkg *packages.Package) error {
	if funcLit, ok := expr.(*ast.FuncLit); ok {
		return e.handleFuncLit(funcLit, curPkg, false)
	}
	name, sig, realPkg, err := getFuncMeta(expr, curPkg, e.pkgMap)
	if err != nil {
		return err
	}
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := e.checkGenerationVisibility(obj); err != nil {
			return err
		}
	}
	genericStr, err := e.extractGenericArgStr(expr, curPkg)
	if err != nil {
		return err
	}
	alias := e.collectPkgAlias(realPkg)

	res := sig.Results()
	switch res.Len() {
	case 0:
		return fmt.Errorf("func %s has no return", name)
	case 1:
		// ok
	case 2:
		if !isErrorType(res.At(1).Type()) {
			return fmt.Errorf("func %s: second return value must be error, got %s", name, res.At(1).Type().String())
		}
	default:
		return fmt.Errorf("func %s: too many return values (%d), only (T) or (T, error) are allowed "+
			"(if you need to provide multiple types, define a plain struct that bundles them and return that struct)", name, res.Len())
	}

	retType := e.getTypeFullName(res.At(0).Type())
	hasErr := sigHasError(sig)
	instanceName := e.extractNamedReturn(sig)

	item := e.newExtractedItem(name, realPkg, alias, hasErr)
	item.RetType = retType
	item.Params = e.buildExtractedParams(sig) // 保留参数名
	item.GenericArgsStr = genericStr
	item.InstanceName = instanceName

	pos := curPkg.Fset.Position(expr.Pos())
	relPath := e.relPath(pos.Filename)
	item.Position = e.ConditionalDebugf(func() bool { return true }, "%s:%d", relPath, pos.Line)

	keyNamed := retType
	if instanceName != "" {
		keyNamed = retType + ":" + instanceName
	}
	// 检查命名键冲突
	if oldIdx, exists := e.globalProviderMap[keyNamed]; exists {
		oldDesc := e.describeItem(oldIdx)
		currentDesc := e.describeItemByIt(item)
		return fmt.Errorf("duplicate binding for %s with name %q:\n\tprevious: %s\n\tcurrent: %s",
			retType, instanceName, oldDesc, currentDesc)
	}
	if instanceName == "" {
		if oldIdx, exists := e.globalProviderMap[retType]; exists {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("duplicate binding for %s (default):\n\tprevious: %s\n\tcurrent: %s",
				retType, oldDesc, currentDesc)
		}
	}

	idx := len(e.items)
	e.items = append(e.items, item)
	if instanceName != "" {
		e.globalProviderMap[keyNamed] = idx
	} else {
		e.globalProviderMap[retType] = idx
	}
	return nil
}

func (e *Extractor) processArgs(args []ast.Expr, pkg *packages.Package, handler func(ast.Expr, *packages.Package) error) error {
	for _, arg := range args {
		if err := handler(arg, pkg); err != nil {
			return err
		}
	}
	return nil
}

func findFuncDecl(pkg *packages.Package, name string) *ast.FuncDecl {
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if ok && fd.Name.Name == name {
				return fd
			}
		}
	}
	return nil
}

func (e *Extractor) findSingleModuleCall(body *ast.BlockStmt, info *types.Info, funcName string) (*ast.CallExpr, error) {
	var moduleCalls []*ast.CallExpr
	var moduleInControl []bool
	var controlDepth int

	astutil.Apply(body,
		func(c *astutil.Cursor) bool {
			switch c.Node().(type) {
			case *ast.IfStmt, *ast.SwitchStmt, *ast.SelectStmt, *ast.ForStmt, *ast.RangeStmt:
				controlDepth++
			}
			if call, ok := c.Node().(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					obj := info.ObjectOf(sel.Sel)
					if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == diPkgPath && obj.Name() == "Module" {
						moduleCalls = append(moduleCalls, call)
						moduleInControl = append(moduleInControl, controlDepth > 0)
					}
				}
			}
			return true
		},
		func(c *astutil.Cursor) bool {
			switch c.Node().(type) {
			case *ast.IfStmt, *ast.SwitchStmt, *ast.SelectStmt, *ast.ForStmt, *ast.RangeStmt:
				controlDepth--
			}
			return true
		},
	)

	switch len(moduleCalls) {
	case 0:
		return nil, fmt.Errorf("function %s does not contain dig.Module", funcName)
	case 1:
		if moduleInControl[0] {
			return nil, fmt.Errorf("function %s contains dig.Module inside control flow (if/switch/for/select), which is not supported; pass it as a parameter to the function (preferred) or move it to package level", funcName)
		}
		return moduleCalls[0], nil
	default:
		return nil, fmt.Errorf("function %s contains multiple dig.Module calls; only one is allowed", funcName)
	}
}

func (e *Extractor) extractOptionsFromFuncCall(call *ast.CallExpr, curPkg *packages.Package) error {
	obj := resolveFunctionObject(call, curPkg)
	if obj == nil {
		return fmt.Errorf("cannot resolve function call; ensure it is a named function or method, not a literal or variable")
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return fmt.Errorf("resolved object is not a function")
	}
	fnPkg := fn.Pkg()
	if fnPkg == nil {
		return fmt.Errorf("function has no package")
	}
	// ✅ 检查生成时可见性（对目标包）
	if err := e.checkGenerationVisibility(obj); err != nil {
		return err
	}
	subPkg, ok := e.pkgMap[fnPkg.Path()]
	if !ok {
		return fmt.Errorf("package %s not loaded", fnPkg.Path())
	}
	funcDecl := findFuncDecl(subPkg, fn.Name())
	if funcDecl == nil || funcDecl.Body == nil {
		return fmt.Errorf("function %s has no body", fn.Name())
	}

	modCall, err := e.findSingleModuleCall(funcDecl.Body, subPkg.TypesInfo, fn.Name())
	if err != nil {
		return err
	}

	for _, arg := range modCall.Args {
		if err := e.extractOptions(arg, subPkg, subPkg); err != nil {
			return err
		}
	}
	return nil
}

// 导出方法
func (e *Extractor) ExtractOptions(expr ast.Expr, curPkg, realPkg *packages.Package) error {
	return e.extractOptions(expr, curPkg, realPkg)
}

func (e *Extractor) BuildFinalNodes() ([]model.Node, error) {
	e.populateUsedPkgs()
	return e.buildFinalNodes()
}
func (e *Extractor) resolveProvider(arg ExtractedArg, it extractedItem) (int, error) {
	if idx, ok := e.resolveProviderIndex(arg); ok {
		return idx, nil
	}
	return 0, e.buildProviderNotFoundError(arg.TypeString, e.getRequiredInstanceName(arg), it)
}

func (e *Extractor) resolveArgNames(it extractedItem, varNames []string) []string {
	argNames := make([]string, len(it.Params))
	for j, arg := range it.Params {
		if arg.IsContext {
			argNames[j] = ""
			continue
		}
		idx, ok := e.resolveProviderIndex(arg)
		if !ok {
			panic(fmt.Sprintf("no provider for type %s", arg.TypeString))
		}
		argNames[j] = varNames[idx]
	}
	return argNames
}

// resolveProviderIndex 根据参数查找对应的 provider 索引（核心查找逻辑）
func (e *Extractor) resolveProviderIndex(arg ExtractedArg) (int, bool) {
	requiredName := e.getRequiredInstanceName(arg)
	key := arg.TypeString
	if requiredName != "" {
		key = arg.TypeString + ":" + requiredName
	}
	if idx, ok := e.globalProviderMap[key]; ok {
		return idx, true
	}
	if requiredName != "" {
		if idx, ok := e.globalProviderMap[arg.TypeString]; ok {
			return idx, true
		}
	}
	return 0, false
}

// getAvailableProviders 返回该类型所有可用的提供者名称列表
func (e *Extractor) getAvailableProviders(typeString string) []string {
	var names []string
	for key := range e.globalProviderMap {
		if after, ok := strings.CutPrefix(key, typeString+":"); ok {
			name := after
			if name != "" {
				names = append(names, name)
			}
		}
	}
	// 检查是否有默认提供者
	if _, ok := e.globalProviderMap[typeString]; ok {
		names = append(names, "(default)")
	}
	sort.Strings(names)
	return names
}

// checkMethodVisibilityInClosure 检查闭包体中的方法调用是否对目标包可见
func (e *Extractor) checkMethodVisibilityInClosure(body *ast.BlockStmt, pkg *packages.Package) error {
	var err error
	ast.Inspect(body, func(n ast.Node) bool {
		// 检查是否是方法调用: CallExpr.Fun 是 SelectorExpr
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// 获取被调用的方法对象
		obj := pkg.TypesInfo.ObjectOf(sel.Sel)
		if obj == nil {
			return true
		}
		// 检查可见性
		if visErr := e.checkGenerationVisibility(obj); visErr != nil {
			err = visErr
			return false
		}
		return true
	})
	return err
}

// checkGenerationVisibility 检查函数对目标包（dig.Build 所在包）是否可见
// 如果函数定义包与目标包相同，或函数是导出的，则可见
// 否则返回错误，提示用户该函数无法在目标包中使用
func (e *Extractor) checkGenerationVisibility(obj types.Object) error {
	if obj == nil {
		return nil
	}

	// 获取定义包
	var pkg *types.Package
	var name string

	switch o := obj.(type) {
	case *types.Func:
		pkg = o.Pkg()
		name = o.Name()
	case *types.Var:
		pkg = o.Pkg()
		name = o.Name()
	case *types.Const:
		pkg = o.Pkg()
		name = o.Name()
	default:
		return nil
	}

	if pkg == nil {
		return nil
	}
	if pkg.Path() == e.mainPkgPath {
		return nil
	}
	if isExported(name) {
		return nil
	}

	return fmt.Errorf("%s %q is private in package %s and cannot be used from package %s (generation target)",
		strings.ToLower(strings.TrimPrefix(fmt.Sprintf("%T", obj), "*types.")),
		name, pkg.Path(), e.mainPkgPath)
}

// buildProviderNotFoundError 构造友好的错误信息
func (e *Extractor) buildProviderNotFoundError(typeString, requiredName string, it extractedItem) error {
	available := e.getAvailableProviders(typeString)

	var hint string
	var fix string

	if len(available) == 0 {
		hint = " (no provider for this type at all)"
		fix = "\n  💡 Fix: add a provider for " + typeString + " via dig.Provide or dig.Supply"
	} else {
		hint = " (available: " + strings.Join(available, ", ") + ")"

		// 检查是否有默认提供者
		var namedOnly []string
		for _, name := range available {
			if name == "(default)" {

			} else {
				namedOnly = append(namedOnly, name)
			}
		}

		if requiredName == "" {
			// 请求默认但只有命名提供者
			if len(namedOnly) == 1 {
				fix = "\n  💡 Fix: rename parameter to '" + namedOnly[0] + "' (matches the only named provider), or remove the name from the provider's return value to make it default"
			} else {
				fix = "\n  💡 Fix: add a default provider via dig.Provide(func() " + typeString + " { ... }), or rename parameter to one of the available names"
			}
		} else if len(namedOnly) == 1 && namedOnly[0] == requiredName {
			// 这个情况实际上不会发生，因为精确匹配已经成功了
			// 保留，但实际不会用到
			fix = "\n  💡 Fix: check if the provider is accessible from the current package"
		} else if len(namedOnly) == 1 {
			// 只有一个命名提供者，用户请求了不同的名字
			fix = "\n  💡 Fix: rename parameter to '" + namedOnly[0] + "' (matches the only named provider), or remove the name from the provider's return value to make it default"
		} else {
			// 多个命名提供者
			fix = "\n  💡 Fix: rename parameter to one of the available names, or add a default provider"
		}
	}

	funcName := model.FullFuncName(it.Pkg.PkgPath, it.FuncName)
	if it.IsClosure {
		funcName = it.FuncName + " (closure)"
	}

	return fmt.Errorf("no provider for type %s with name %q required by %s at %s%s%s",
		typeString, requiredName, funcName, it.Position, hint, fix)
}

// ---------- buildDependencyGraph 修改 ----------
func (e *Extractor) buildDependencyGraph(items []extractedItem) ([][]int, []int, error) {
	n := len(items)
	adj := make([][]int, n)
	indeg := make([]int, n)

	for i, it := range items {
		if it.IsSupply {
			continue
		}
		for _, arg := range it.Params {
			if arg.IsContext {
				continue
			}
			if it.IsClosure && arg.IsConst {
				continue
			}

			providerIdx, err := e.resolveProvider(arg, it)
			if err != nil {
				return nil, nil, err
			}

			adj[providerIdx] = append(adj[providerIdx], i)
			indeg[i]++
		}
	}
	return adj, indeg, nil
}

func topologicalSort(n int, adj [][]int, indeg []int) ([]int, error) {
	queue := make([]int, 0, n)
	for i := range n {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	order := make([]int, 0, n)
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		order = append(order, u)
		for _, v := range adj[u] {
			indeg[v]--
			if indeg[v] == 0 {
				queue = append(queue, v)
			}
		}
	}
	if len(order) != n {
		return nil, fmt.Errorf("circular dependency")
	}
	return order, nil
}

func (e *Extractor) findCycle(adj [][]int) ([]int, error) {
	n := len(adj)
	state := make([]int, n) // 0=未访问, 1=访问中, 2=已处理
	parent := make([]int, n)
	for i := range n {
		if state[i] == 0 {
			stack := []int{i}
			state[i] = 1
			parent[i] = -1
			for len(stack) > 0 {
				u := stack[len(stack)-1]
				found := false
				for _, v := range adj[u] {
					if state[v] == 0 {
						state[v] = 1
						parent[v] = u
						stack = append(stack, v)
						found = true
						break
					} else if state[v] == 1 {
						cycle := []int{v}
						for cur := u; cur != v; cur = parent[cur] {
							cycle = append(cycle, cur)
						}
						return cycle, nil
					}
				}
				if !found {
					state[u] = 2
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	return nil, fmt.Errorf("no cycle found")
}

// describeItemByIt 直接根据 extractedItem 生成描述，不依赖索引
func (e *Extractor) describeItemByIt(it extractedItem) string {
	if it.IsSupply {
		kind := "Supply"
		if it.FuncName != "" {
			kind += fmt.Sprintf(": argument '%s'", it.FuncName)
		} else if it.Expr != nil {
			var buf strings.Builder
			_ = printer.Fprint(&buf, it.Pkg.Fset, it.Expr)
			kind += ": " + buf.String()
		} else {
			kind += ": <anonymous>"
		}
		desc := fmt.Sprintf("%s -> %s", kind, it.RetType)
		if it.Position != "" {
			desc += fmt.Sprintf(" at %s", it.Position)
		}
		if it.InstanceName != "" {
			desc += fmt.Sprintf(" (name: %q)", it.InstanceName)
		}
		return desc
	}
	var kind string
	if it.IsInvoke {
		kind = "Invoke"
	} else {
		kind = "Provide"
	}
	funcName := model.FullFuncName(it.Pkg.PkgPath, it.FuncName)
	if it.IsClosure {
		funcName = it.FuncName + " (closure)"
	}
	desc := fmt.Sprintf("%s: %s", kind, funcName)
	if it.RetType != "" {
		desc += fmt.Sprintf(" -> %s", it.RetType)
	}
	if it.Position != "" {
		desc += fmt.Sprintf(" at %s", it.Position)
	}
	if it.InstanceName != "" {
		desc += fmt.Sprintf(" (name: %q)", it.InstanceName)
	}
	return desc
}

// ---------- describeItem 使用 Params ----------
func (e *Extractor) describeItem(idx int) string {
	if idx < 0 || idx >= len(e.items) {
		return fmt.Sprintf("invalid index %d", idx)
	}
	return e.describeItemByIt(e.items[idx])
}
func (e *Extractor) formatCycleError(cycle []int) error {
	cycleDesc := functional.Map(cycle, e.describeItem)
	cycleInfo := strings.Join(cycleDesc, " -> ")
	return fmt.Errorf("circular dependency detected: %s", cycleInfo)
}

func (e *Extractor) computeOrder(adj [][]int, indeg []int) ([]int, error) {
	n := len(adj)
	indegCopy := make([]int, n)
	copy(indegCopy, indeg)

	order, err := topologicalSort(n, adj, indegCopy)
	if err != nil {
		cycle, cycleErr := e.findCycle(adj)
		if cycleErr != nil {
			return nil, fmt.Errorf("circular dependency (failed to locate cycle): %w", err)
		}
		return nil, e.formatCycleError(cycle)
	}
	return order, nil
}

func (e *Extractor) buildFinalNodes() ([]model.Node, error) {
	items := e.items
	adj, indeg, err := e.buildDependencyGraph(items)
	if err != nil {
		return nil, err
	}
	order, err := e.computeOrder(adj, indeg)
	if err != nil {
		return nil, err
	}
	order = e.reorderInvokes(order, items)
	varNames := e.assignVarNames(order, items)
	nodes := e.buildNodes(order, items, varNames)
	return nodes, nil
}

// ---------- baseNode 构建 []model.Arg ----------
func (e *Extractor) baseNode(it extractedItem, name string, argNames []string) model.Node {
	args := make([]model.Arg, len(it.Params))
	for i, arg := range it.Params {
		args[i] = model.Arg{
			Name:       argNames[i],
			IsConst:    arg.IsConst,
			ConstValue: arg.ConstValue,
			IsContext:  arg.IsContext,
		}
	}
	return model.Node{
		Name:        name,
		PkgPath:     it.Pkg.PkgPath,
		FuncPkg:     it.PkgAlias,
		Args:        args,
		GenericArgs: it.GenericArgsStr,
	}
}

// ---------- buildInvokeNode 使用 baseNode ----------
func (e *Extractor) buildInvokeNode(it extractedItem, argNames []string) model.Node {
	node := e.baseNode(it, "", argNames)
	node.IsInvoke = true
	node.HasError = it.HasError
	node.IsClosure = it.IsClosure
	node.Func = it.FuncName
	node.FuncPkg = it.PkgAlias
	if it.IsClosure {
		node.PkgPath = e.mainPkgPath
		closureDef, usedPkgs, err := e.generateClosureDef(&it)
		if err != nil {
			panic(err)
		}
		node.ClosureDef = closureDef
		node.UsedPkgs = usedPkgs
	}
	return node
}

type pair struct {
	path  string
	alias string
}

func (e *Extractor) replacePkgPathWithAlias(typeStr string) string {
	var prefix strings.Builder
	for {
		if strings.HasPrefix(typeStr, "*") {
			prefix.WriteString("*")
			typeStr = typeStr[1:]
		} else if strings.HasPrefix(typeStr, "[]") {
			prefix.WriteString("[]")
			typeStr = typeStr[2:]
		} else {
			break
		}
	}

	// 主包路径前缀替换（一次 ReplaceAll 等价于原循环）
	mainPrefix := e.mainPkgPath + "."
	typeStr = strings.ReplaceAll(typeStr, mainPrefix, "")

	pairs := functional.MapEntries(e.pkgAliasMap, func(path, alias string) pair {
		return pair{path, alias}
	})
	sort.Slice(pairs, func(i, j int) bool {
		return len(pairs[i].path) > len(pairs[j].path)
	})

	// 单次遍历（无需循环，因为替换后不会产生新的包路径）
	for _, p := range pairs {
		typeStr = strings.ReplaceAll(typeStr, p.path+".", p.alias+".")
	}

	return prefix.String() + typeStr
}

// ---------- buildParamListAndFreeVarMap 使用新字段 ----------
func (e *Extractor) buildParamListAndFreeVarMap(it *extractedItem, usedPkgs map[string]bool) ([]string, map[string]string) {
	var paramList []string
	freeVarMap := make(map[string]string)

	// 闭包参数
	for _, arg := range it.ClosureParams {
		typStr := e.replacePkgPathWithAlias(arg.TypeString)
		paramList = append(paramList, arg.Name+" "+typStr)
		e.addPkgToUsed(arg.Type, usedPkgs)
	}

	// 自由变量（从 Params 中取闭包参数之后的部分）
	startIdx := len(it.ClosureParams)
	for i := startIdx; i < len(it.Params); i++ {
		arg := it.Params[i]
		if arg.IsConst {
			continue
		}
		paramName := "p" + strconv.Itoa(i-startIdx)
		typStr := e.replacePkgPathWithAlias(arg.TypeString)
		paramList = append(paramList, paramName+" "+typStr)
		freeVarMap[arg.Name] = paramName
		e.addPkgToUsed(arg.Type, usedPkgs)
	}

	return paramList, freeVarMap
}

func (e *Extractor) typePkg(typ types.Type) *types.Package {
	switch t := typ.(type) {
	case *types.Named:
		return t.Obj().Pkg()
	case *types.Pointer:
		return e.typePkg(t.Elem())
	case *types.Slice:
		return e.typePkg(t.Elem())
	case *types.Array:
		return e.typePkg(t.Elem())
	case *types.Map:
		return nil
	case *types.Chan:
		return e.typePkg(t.Elem())
	default:
		return nil
	}
}

func (e *Extractor) replaceFreeVarsInBody(body *ast.BlockStmt, freeVarMap map[string]string) *ast.BlockStmt {
	newNode := astutil.Apply(body,
		func(c *astutil.Cursor) bool {
			if ident, ok := c.Node().(*ast.Ident); ok {
				if newName, ok := freeVarMap[ident.Name]; ok {
					c.Replace(ast.NewIdent(newName))
					return false
				}
			}
			return true
		},
		nil,
	)
	if blk, ok := newNode.(*ast.BlockStmt); ok {
		return blk
	}
	return body
}

func (e *Extractor) collectTypeNameAndUsedPkgs(body *ast.BlockStmt, pkg *packages.Package, usedPkgs map[string]bool) map[string]string {
	typeNameMap := make(map[string]string)
	ast.Inspect(body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.ObjectOf(ident)
		if obj == nil {
			return true
		}

		// 处理类型名（如 config.Config）
		if typeName, ok := obj.(*types.TypeName); ok {
			pkgObj := typeName.Pkg()
			if pkgObj != nil && pkgObj.Path() != e.mainPkgPath {
				alias := e.ensureAlias(pkgObj.Path())
				if alias != "" {
					typeNameMap[ident.Name] = alias + "." + ident.Name
					usedPkgs[pkgObj.Path()] = true
				}
			}
			return true
		}

		// 处理包名（如 alias.ParseAliasType 中的 alias）
		if pkgName, ok := obj.(*types.PkgName); ok {
			pkgPath := pkgName.Imported().Path()
			if pkgPath != "" && pkgPath != e.mainPkgPath {
				usedPkgs[pkgPath] = true
			}
			return true
		}

		return true
	})
	return typeNameMap
}

func (e *Extractor) applyTypeAliasReplacements(bodyStr string, typeNameMap map[string]string) string {
	bodyStr = replaceTypeNames(bodyStr, typeNameMap)
	return e.replacePkgPathWithAlias(bodyStr)
}

// ---------- generateClosureDef 使用新字段 ----------
func (e *Extractor) generateClosureDef(it *extractedItem) (string, []string, error) {
	allTypes := make([]types.Type, 0, len(it.ClosureParams)+len(it.FreeTypes)+1)
	for _, arg := range it.ClosureParams {
		allTypes = append(allTypes, arg.Type)
	}
	allTypes = append(allTypes, it.FreeTypes...)
	if it.ClosureLit.Type.Results != nil && len(it.ClosureLit.Type.Results.List) > 0 {
		retExpr := it.ClosureLit.Type.Results.List[0].Type
		if typ := it.Pkg.TypesInfo.TypeOf(retExpr); typ != nil {
			allTypes = append(allTypes, typ)
		}
	}

	usedPkgs := make(map[string]bool)
	for _, t := range allTypes {
		if pkg := e.typePkg(t); pkg != nil && pkg.Path() != e.mainPkgPath {
			usedPkgs[pkg.Path()] = true
			e.ensureAlias(pkg.Path())
		}
	}

	paramList, freeVarMap := e.buildParamListAndFreeVarMap(it, usedPkgs)

	paramStr := strings.Join(paramList, ", ")

	rewrittenBody := e.replaceFreeVarsInBody(it.ClosureLit.Body, freeVarMap)

	typeNameMap := e.collectTypeNameAndUsedPkgs(rewrittenBody, it.Pkg, usedPkgs)

	var bodyBuf bytes.Buffer
	if err := printer.Fprint(&bodyBuf, it.Pkg.Fset, rewrittenBody); err != nil {
		return "", nil, fmt.Errorf("printer print closure body failed: %w", err)
	}
	bodyStr := bodyBuf.String()
	bodyStr = e.applyTypeAliasReplacements(bodyStr, typeNameMap)
	// 将左大括号后的多个换行压缩为单个换行
	bodyStr = regexp.MustCompile(`\{\n{2,}`).ReplaceAllString(bodyStr, "{\n")
	// 并将多余的空行（连续 3 个以上换行）压缩为两个换行
	bodyStr = regexp.MustCompile(`\n{3,}`).ReplaceAllString(bodyStr, "\n\n")

	retStr := formatResultList(it.ClosureLit.Type.Results, it.Pkg, e)

	// 构建闭包定义
	def := e.buildClosureDefString(it.FuncName, paramStr, retStr, bodyStr)
	if it.SourceComment != "" {
		def = it.SourceComment + "\n" + def
	}
	usedList := functional.Keys(usedPkgs)
	comment := e.ConditionalDebugf(func() bool { return it.Pkg.PkgPath != e.mainPkgPath }, "// original package: %s\n", it.Pkg.PkgPath)
	def = comment + def
	return def, usedList, nil
}

// ensureAlias 确保指定包路径在 pkgAliasMap 中存在别名，如果不存在则生成并缓存。
// 若包在 pkgMap 中，则调用 collectPkgAlias（会基于策略和冲突处理生成）；
// 否则使用路径最后一段作为别名并缓存。
// 返回别名（若包路径为主包或空，返回空字符串）。
func (e *Extractor) ensureAlias(pkgPath string) string {
	if pkgPath == "" || pkgPath == e.mainPkgPath {
		return ""
	}
	if alias, ok := e.pkgAliasMap[pkgPath]; ok {
		return alias
	}
	if pkg, ok := e.pkgMap[pkgPath]; ok {
		return e.collectPkgAlias(pkg)
	}
	// 不在 pkgMap 中，使用策略生成唯一别名
	existing := make(map[string]bool)
	for _, a := range e.pkgAliasMap {
		existing[a] = true
	}
	alias := e.aliasStrategy.GenerateAlias(pkgPath, existing)
	e.pkgAliasMap[pkgPath] = alias
	return alias
}

// formatResultList 从 ast.FieldList 生成返回值字符串
// 例如：单个无名返回值 -> "string"
//
//	多个或有名字的返回值 -> "(str string, err error)"
func formatResultList(fieldList *ast.FieldList, pkg *packages.Package, e *Extractor) string {
	if fieldList == nil || len(fieldList.List) == 0 {
		return ""
	}
	var parts []string
	for _, field := range fieldList.List {
		typ := pkg.TypesInfo.TypeOf(field.Type)
		typeStr := e.replacePkgPathWithAlias(e.getTypeFullName(typ))
		if len(field.Names) == 0 {
			// 无名返回值
			parts = append(parts, typeStr)
		} else {
			for _, name := range field.Names {
				parts = append(parts, name.Name+" "+typeStr)
			}
		}
	}
	// 如果只有一个返回值且没有名字，直接返回类型（不带括号）
	if len(parts) == 1 && len(fieldList.List) == 1 && len(fieldList.List[0].Names) == 0 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
func (e *Extractor) buildClosureDefString(funcName, paramStr, retStr, bodyStr string) string {
	if retStr != "" {
		return fmt.Sprintf("func %s(%s) %s %s", funcName, paramStr, retStr, bodyStr)
	}
	return fmt.Sprintf("func %s(%s) %s", funcName, paramStr, bodyStr)
}

func (e *Extractor) buildNodes(order []int, items []extractedItem, varNames []string) []model.Node {
	var final []model.Node
	for _, i := range order {
		it := items[i]
		argNames := e.resolveArgNames(it, varNames)
		switch {
		case it.IsInvoke:
			final = append(final, e.buildInvokeNode(it, argNames))
		case it.IsSupply:
			final = append(final, e.buildSupplyNode(it, varNames[i]))
		default:
			final = append(final, e.buildProviderNode(it, argNames, varNames[i]))
		}
	}
	return final
}

// ---------- buildProviderNode 使用 baseNode ----------
func (e *Extractor) buildProviderNode(it extractedItem, argNames []string, name string) model.Node {
	node := e.baseNode(it, name, argNames)
	node.RetType = it.RetType
	node.HasError = it.HasError
	node.IsClosure = it.IsClosure
	node.Func = it.FuncName
	node.FuncPkg = it.PkgAlias
	if it.IsClosure {
		node.PkgPath = e.mainPkgPath
		closureDef, usedPkgs, err := e.generateClosureDef(&it)
		if err != nil {
			panic(err)
		}
		node.ClosureDef = closureDef
		node.UsedPkgs = usedPkgs
	}
	return node
}

func (e *Extractor) buildSupplyNode(it extractedItem, name string) model.Node {
	var buf strings.Builder
	_ = printer.Fprint(&buf, it.Pkg.Fset, it.Expr)
	return model.Node{
		Name:     name,
		IsSupply: true,
		Value:    buf.String(),
		FuncPkg:  it.PkgAlias,
		PkgPath:  it.Pkg.PkgPath,
		RetType:  it.RetType,
		UsedPkgs: it.UsedPkgs,
		Comment:  it.SourceComment,
	}
}

func (e *Extractor) assignVarNames(order []int, items []extractedItem) []string {
	n := len(items)
	varNames := make([]string, n)
	vIdx := 0
	for _, i := range order {
		if !items[i].IsInvoke {
			varNames[i] = fmt.Sprintf("v%d", vIdx)
			vIdx++
		}
	}
	return varNames
}

func (e *Extractor) reorderInvokes(order []int, items []extractedItem) []int {
	var nonInvokeOrder []int
	var preservedInvokeOrder []int
	for idx, it := range items {
		if it.IsInvoke {
			preservedInvokeOrder = append(preservedInvokeOrder, idx)
		}
	}
	for _, idx := range order {
		if !items[idx].IsInvoke {
			nonInvokeOrder = append(nonInvokeOrder, idx)
		}
	}
	return append(nonInvokeOrder, preservedInvokeOrder...)
}

func findDigCallInBlock(block *ast.BlockStmt, info *types.Info, methodName string) *ast.CallExpr {
	var result *ast.CallExpr
	ast.Inspect(block, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		obj := info.ObjectOf(sel.Sel)
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == diPkgPath && obj.Name() == methodName {
			result = call
			return false
		}
		return true
	})
	return result
}

func addExternalParams(extractor *Extractor, target *model.GenTarget, pkg *packages.Package) error {
	params := target.Node.Type.Params
	if params == nil {
		return nil
	}
	seenTypes := make(map[string]bool)
	pos := pkg.Fset.Position(target.Node.Pos())
	relPath := extractor.relPath(pos.Filename)
	for _, field := range params.List {
		for _, name := range field.Names {
			typ := pkg.TypesInfo.TypeOf(field.Type)
			if typ == nil {
				return fmt.Errorf("cannot resolve type of parameter %s", name.Name)
			}
			retType := extractor.getTypeFullName(typ)
			if seenTypes[retType] {
				return fmt.Errorf("duplicate parameter type %q (parameter %s)", retType, name.Name)
			}
			seenTypes[retType] = true
			sourceComment := extractor.ConditionalDebugf(func() bool { return true }, "// supplied from function '%s' argument '%s' (type %s) at %s:%d", target.Node.Name.Name, name.Name, retType, relPath, pos.Line)
			// 使用原始标识符保持类型信息
			expr := ast.NewIdent(name.Name)
			item := extractedItem{
				Pkg:           pkg,
				PkgAlias:      "",
				FuncName:      name.Name,
				IsSupply:      true,
				RetType:       retType,
				Expr:          expr,
				UsedPkgs:      extractor.collectUsedPkgsFromType(typ),
				SourceComment: sourceComment,
				Position:      fmt.Sprintf("%s:%d", relPath, pos.Line),
				InstanceName:  "", // 外部参数作为默认提供者，不参与命名匹配
			}
			extractor.items = append(extractor.items, item)
			idx := len(extractor.items) - 1
			extractor.globalProviderMap[retType] = idx
		}
	}
	return nil
}

func isContextFunc(typ types.Type) bool {
	sig, ok := typ.(*types.Signature)
	if !ok {
		return false
	}
	params := sig.Params()
	if params.Len() != 1 {
		return false
	}
	if !isContextType(params.At(0).Type()) {
		return false
	}
	results := sig.Results()
	if results.Len() != 1 {
		return false
	}
	return isErrorType(results.At(0).Type())
}

func validateReturnType(fnDecl *ast.FuncDecl, info *types.Info) error {
	if fnDecl.Type.Results == nil || len(fnDecl.Type.Results.List) == 0 {
		return fmt.Errorf("function %q: must have a return value of type func(context.Context) error", fnDecl.Name.Name)
	}
	if len(fnDecl.Type.Results.List) > 1 {
		return fmt.Errorf("function %q: only a single return value allowed, expected func(context.Context) error", fnDecl.Name.Name)
	}
	resField := fnDecl.Type.Results.List[0]
	if len(resField.Names) > 0 {
		return fmt.Errorf("function %q: named return value is not allowed, expected func(context.Context) error", fnDecl.Name.Name)
	}
	retType := info.TypeOf(resField.Type)
	if retType == nil {
		return fmt.Errorf("function %q: failed to resolve return type", fnDecl.Name.Name)
	}
	if !isContextFunc(retType) {
		return fmt.Errorf("function %q: invalid return type %q, expected func(context.Context) error", fnDecl.Name.Name, retType.String())
	}
	return nil
}

// 导出函数
func AddExternalParams(extractor *Extractor, target *model.GenTarget, pkg *packages.Package) error {
	return addExternalParams(extractor, target, pkg)
}

func FindDigCallInBlock(block *ast.BlockStmt, info *types.Info, methodName string) *ast.CallExpr {
	return findDigCallInBlock(block, info, methodName)
}

func ValidateReturnType(fnDecl *ast.FuncDecl, info *types.Info) error {
	return validateReturnType(fnDecl, info)
}

func FindBuildCall(fn *ast.FuncDecl, info *types.Info) *ast.CallExpr {
	if fn.Body == nil {
		return nil
	}
	return findDigCallInBlock(fn.Body, info, "Build")
}

func (e *Extractor) PkgAliasMap() map[string]string {
	return e.pkgAliasMap
}

type importInfo struct {
	filePath string
	pkgPath  string
	alias    string
	isMain   bool
}

func (e *Extractor) collectAllImportInfos() []importInfo {
	var infos []importInfo
	for _, p := range e.pkgMap {
		isMain := p.PkgPath == e.mainPkgPath
		for _, f := range p.Syntax {
			filePos := p.Fset.Position(f.Pos())
			filePath := filePos.Filename
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if imp.Name != nil {
					alias := imp.Name.Name
					if alias != "." && alias != "_" {
						infos = append(infos, importInfo{
							filePath: filePath,
							pkgPath:  path,
							alias:    alias,
							isMain:   isMain,
						})
					}
				}
			}
		}
	}
	return infos
}

func (e *Extractor) loadImportAliases() {
	infos := e.collectAllImportInfos()
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].filePath != infos[j].filePath {
			return infos[i].filePath < infos[j].filePath
		}
		return infos[i].pkgPath < infos[j].pkgPath
	})
	for _, info := range infos {
		if info.alias == "." || info.alias == "_" {
			continue
		}
		if _, exists := e.importAliasMap[info.pkgPath]; !exists {
			e.importAliasMap[info.pkgPath] = info.alias
		}
	}
}
