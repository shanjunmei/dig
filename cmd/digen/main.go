package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"
	"go/types"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

const (
	diPkgPath   = "github.com/shanjunmei/dig"
	identDigApp = "App"
	tagBuild    = "digen"
)

type UnusedMode int

const (
	UnusedModeError UnusedMode = iota
	UnusedModeIgnore
	UnusedModeDrop
)

var (
	outputFile    = flag.String("out", "dig_gen.go", "output file name")
	unusedModeStr = flag.String("unused", "error", "behavior for unused providers: error, ignore, drop")
	debugEnabled  bool
)

type GenTarget struct {
	FuncName string
	Node     *ast.FuncDecl
	File     string
}

type Node struct {
	Name      string
	Func      string
	FuncPkg   string
	RetType   string
	Args      []string
	IsInvoke  bool
	IsSupply  bool
	Value     string
	HasError  bool
	IsClosure bool

	ClosureDef string
	UsedPkgs   []string
	PkgPath    string

	IsConstArg     []bool
	ConstLitValues []string
	IsContextArg   []bool
}

type extractedItem struct {
	FuncName string
	RetType  string
	ArgTypes []string
	IsInvoke bool
	IsSupply bool
	Expr     ast.Expr
	Pkg      *packages.Package
	PkgAlias string
	HasError bool

	IsClosure       bool
	ClosureLit      *ast.FuncLit
	FreeVars        []*ast.Ident
	FreeTypes       []types.Type
	FreeTypeStrings []string

	ClosureParamNames []string
	ClosureParamTypes []types.Type

	// 新增：常量信息（仅对自由变量有效）
	IsConstArg     []bool
	ConstLitValues []string
	IsContextArg   []bool
}

type Extractor struct {
	pkgMap            map[string]*packages.Package
	mainPkgPath       string
	items             []extractedItem
	globalProviderMap map[string]int
	pkgAliasMap       map[string]string
	UnusedMode        UnusedMode
	importAliasMap    map[string]string
}

// ----------------------------------------------------------------------------
// 基础辅助函数（无状态）
// ----------------------------------------------------------------------------

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

func findInjectorFunctions(pkg *packages.Package) (*GenTarget, error) {
	var targets []GenTarget
	for idx, f := range pkg.Syntax {
		currentFile := pkg.GoFiles[idx]
		for _, decl := range f.Decls {
			fnDecl, ok := decl.(*ast.FuncDecl)
			if !ok || fnDecl.Body == nil {
				continue
			}
			if findDigCallInBlock(fnDecl.Body, pkg.TypesInfo, "Build") == nil {
				continue
			}
			if err := validateReturnType(fnDecl, pkg.TypesInfo); err != nil {
				return nil, fmt.Errorf("function %q: %v", fnDecl.Name.Name, err)
			}
			targets = append(targets, GenTarget{
				FuncName: fnDecl.Name.Name,
				Node:     fnDecl,
				File:     currentFile,
			})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no function containing dig.Build call found")
	}
	if len(targets) > 1 {
		return nil, fmt.Errorf("multiple functions containing dig.Build call found (only one allowed)")
	}
	return &targets[0], nil
}

// isDigAppPointer 判断类型是否为 *dig.App
func isDigAppPointer(typ types.Type) bool {
	ptr, ok := typ.(*types.Pointer)
	if !ok {
		return false
	}
	elem := ptr.Elem()
	named, ok := elem.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == diPkgPath && obj.Name() == identDigApp
}

func validateReturnType(fnDecl *ast.FuncDecl, info *types.Info) error {
	// 检查返回值是否存在
	if fnDecl.Type.Results == nil || len(fnDecl.Type.Results.List) == 0 {
		return fmt.Errorf("function %q: must have return value, required: *dig.App", fnDecl.Name.Name)
	}
	// 只允许一个返回值
	if len(fnDecl.Type.Results.List) > 1 {
		return fmt.Errorf("function %q: only allow single return value, required: *dig.App", fnDecl.Name.Name)
	}
	resField := fnDecl.Type.Results.List[0]
	// 禁止命名返回值
	if len(resField.Names) > 0 {
		return fmt.Errorf("function %q: named return value is not allowed, required: *dig.App", fnDecl.Name.Name)
	}
	retType := info.TypeOf(resField.Type)
	if retType == nil {
		return fmt.Errorf("function %q: failed to resolve return type", fnDecl.Name.Name)
	}
	if !isDigAppPointer(retType) {
		return fmt.Errorf("function %q: invalid return type %q, required: *dig.App", fnDecl.Name.Name, retType.String())
	}
	return nil
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
func uniqueAlias(pkgPath string, existing map[string]bool) string {
	parts := strings.Split(pkgPath, "/")
	if len(parts) == 0 {
		return "pkg"
	}
	for i := 1; i <= len(parts); i++ {
		alias := strings.Join(parts[len(parts)-i:], "_")
		alias = strings.ReplaceAll(alias, ".", "_")
		alias = strings.ReplaceAll(alias, "-", "_")
		if len(alias) > 0 && alias[0] >= '0' && alias[0] <= '9' {
			alias = "_" + alias
		}
		if !existing[alias] {
			return alias
		}
	}
	fullAlias := strings.ReplaceAll(pkgPath, "/", "_")
	fullAlias = strings.ReplaceAll(fullAlias, ".", "_")
	fullAlias = strings.ReplaceAll(fullAlias, "-", "_")
	if !existing[fullAlias] {
		return fullAlias
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", fullAlias, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

func fullFuncName(pkgAlias, funcName string) string {
	if pkgAlias == "" {
		return funcName
	}
	return pkgAlias + "." + funcName
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
	return fn.Name(), fn.Type().(*types.Signature), realPkg, nil
}

func sigHasError(sig *types.Signature) bool {
	res := sig.Results()
	if res.Len() == 0 {
		return false
	}
	lastTyp := res.At(res.Len() - 1).Type()
	return types.Identical(lastTyp, types.Universe.Lookup("error").Type())
}

func resolveFunctionObject(call *ast.CallExpr, curPkg *packages.Package) types.Object {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		obj := curPkg.TypesInfo.ObjectOf(fun)
		if obj != nil {
			debugf("[DEBUG resolve] ident %q -> %v (pkg=%v)", fun.Name, obj, obj.Pkg())
		}
		return obj
	case *ast.SelectorExpr:
		obj := curPkg.TypesInfo.ObjectOf(fun.Sel)
		if obj != nil {
			debugf("[DEBUG resolve] selector %s.%s -> %v (pkg=%v)", fun.X, fun.Sel.Name, obj, obj.Pkg())
		}
		return obj
	default:
		return nil
	}
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

func collectAllPackages(rootPkgs []*packages.Package) map[string]*packages.Package {
	pkgMap := make(map[string]*packages.Package)
	var collect func(*packages.Package)
	collect = func(p *packages.Package) {
		if _, exists := pkgMap[p.PkgPath]; exists {
			return
		}
		if p.PkgPath == "" {
			return
		}
		pkgMap[p.PkgPath] = p
		for _, impPkg := range p.Imports {
			collect(impPkg)
		}
	}
	for _, rootPkg := range rootPkgs {
		collect(rootPkg)
	}
	return pkgMap
}

func findBuildCall(fn *ast.FuncDecl, info *types.Info) *ast.CallExpr {
	if fn.Body == nil {
		return nil
	}
	return findDigCallInBlock(fn.Body, info, "Build")
}

// ----------------------------------------------------------------------------
// Extractor 方法
// ----------------------------------------------------------------------------
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

// findSingleModuleCall 在函数体中查找唯一的 dig.Module 调用，
// 要求必须恰好有一个，且位于顶层（不在 if/switch/for/select 内）。
// 返回该调用，若不符合则返回错误。
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
			return nil, fmt.Errorf("function %s contains dig.Module inside control flow (if/switch/for/select), which is not supported; move it to top level", funcName)
		}
		return moduleCalls[0], nil
	default:
		return nil, fmt.Errorf("function %s contains multiple dig.Module calls; only one is allowed", funcName)
	}
}
func (e *Extractor) extractOptions(expr ast.Expr, curPkg, realPkg *packages.Package) error {
	expr = ast.Unparen(expr)
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		pos := curPkg.Fset.Position(expr.Pos())
		return fmt.Errorf("at %s: unsupported option expression (must be a direct call to Provide, Invoke, Supply, or Module, got %T)", pos, expr)
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if obj := curPkg.TypesInfo.ObjectOf(sel.Sel); obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == diPkgPath {
			switch obj.Name() {
			case "Provide":
				return e.processProvideArgs(call.Args, realPkg)
			case "Invoke":
				return e.processInvokeArgs(call.Args, realPkg)
			case "Supply":
				return e.processSupplyArgs(call.Args, realPkg)
			case "Module":
				return e.processModuleArgs(call.Args, curPkg, realPkg)
			}
		}
	}
	return e.extractOptionsFromFuncCall(call, curPkg)
}

func (e *Extractor) processProvideArgs(args []ast.Expr, pkg *packages.Package) error {
	for _, arg := range args {
		if err := e.handleProvide(arg, pkg); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extractor) processInvokeArgs(args []ast.Expr, pkg *packages.Package) error {
	for _, arg := range args {
		if err := e.handleInvoke(arg, pkg); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extractor) processSupplyArgs(args []ast.Expr, pkg *packages.Package) error {
	for _, arg := range args {
		if err := e.handleSupply(arg, pkg); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extractor) processModuleArgs(args []ast.Expr, curPkg, realPkg *packages.Package) error {
	for _, arg := range args {
		if err := e.extractOptions(arg, curPkg, realPkg); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extractor) typeQualifier(p *types.Package) string {
	return p.Path()
}

func (e *Extractor) getTypeFullName(typ types.Type) string {
	return types.TypeString(typ, e.typeQualifier)
}

func (e *Extractor) handleProvide(expr ast.Expr, curPkg *packages.Package) error {
	if funcLit, ok := expr.(*ast.FuncLit); ok {
		return e.handleFuncLit(funcLit, curPkg, false)
	}
	name, sig, realPkg, err := getFuncMeta(expr, curPkg, e.pkgMap)
	if err != nil {
		return err
	}
	alias := e.collectPkgAlias(realPkg)

	res := sig.Results()
	switch res.Len() {
	case 0:
		return fmt.Errorf("func %s has no return", name)
	case 1:
		//允许 T
	case 2:
		if !types.Identical(res.At(1).Type(), types.Universe.Lookup("error").Type()) {
			return fmt.Errorf("func %s: second return value must be error, got %s", name, res.At(1).Type().String())
		}
	default:
		return fmt.Errorf("func %s: too many return values (%d), only (T) or (T, error) are allowed. "+
			"If you need to provide multiple types, define a plain struct that bundles them and return that struct.", name, res.Len())
	}

	retType := e.getTypeFullName(res.At(0).Type())
	if _, dup := e.globalProviderMap[retType]; dup {
		return fmt.Errorf("duplicate provide for type %q", retType)
	}
	argTypes := make([]string, sig.Params().Len())
	isContextArg := make([]bool, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		typ := param.Type()
		argTypes[i] = e.getTypeFullName(typ)
		isContextArg[i] = isContextType(typ)
	}
	hasErr := sigHasError(sig)
	idx := len(e.items)
	e.items = append(e.items, extractedItem{
		FuncName:     name,
		RetType:      retType,
		ArgTypes:     argTypes,
		Pkg:          realPkg,
		PkgAlias:     alias,
		HasError:     hasErr,
		IsContextArg: isContextArg,
	})
	e.globalProviderMap[retType] = idx
	return nil
}
func (e *Extractor) handleFuncLit(funcLit *ast.FuncLit, curPkg *packages.Package, isInvoke bool) error {
	typ := curPkg.TypesInfo.TypeOf(funcLit)
	sig, ok := typ.(*types.Signature)
	if !ok {
		return fmt.Errorf("func literal is not a function type")
	}

	paramNames, paramTypes, paramTypeStrs := e.extractClosureParams(funcLit, curPkg)

	freeVars, freeTypes, freeTypeStrs, freeIsConst, freeLitValues, err := e.collectFreeVarsWithConst(funcLit, curPkg)
	if err != nil {
		return err
	}

	argTypes, isConstArg, litValues := e.buildArgTypesAndFlags(paramTypes, paramTypeStrs, freeTypes, freeIsConst, freeLitValues)

	hasErr := sigHasError(sig)
	retType, err := e.determineReturnType(funcLit, sig, isInvoke, curPkg)
	if err != nil {
		return err
	}
	if retType != "" {
		if _, dup := e.globalProviderMap[retType]; dup {
			return fmt.Errorf("duplicate provide for type %q", retType)
		}
	}

	funcName := e.generateFuncName(isInvoke)
	isContextArg := make([]bool, len(argTypes))
	// 先处理闭包参数
	for i, t := range paramTypes {
		if isContextType(t) {
			isContextArg[i] = true
		}
	}
	idx := len(e.items)
	item := e.buildExtractedItem(funcName, argTypes, isInvoke, hasErr, curPkg, funcLit,
		freeVars, freeTypes, freeTypeStrs, paramNames, paramTypes,
		isConstArg, litValues, retType, isContextArg)

	e.items = append(e.items, item)
	if !isInvoke && retType != "" {
		e.globalProviderMap[retType] = idx
	}
	return nil
}

// isContextType 判断类型是否为 context.Context
func isContextType(typ types.Type) bool {
	// 简单方式：比较类型字符串（全限定名）
	return typ.String() == "context.Context"
}

// ---------- 辅助方法 for handleFuncLit ----------
func (e *Extractor) extractClosureParams(funcLit *ast.FuncLit, curPkg *packages.Package) ([]string, []types.Type, []string) {
	var names []string
	var typesList []types.Type
	var typeStrs []string
	if funcLit.Type.Params != nil {
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

func (e *Extractor) buildArgTypesAndFlags(paramTypes []types.Type, paramTypeStrs []string, freeTypes []types.Type, freeIsConst []bool, freeLitValues []string) ([]string, []bool, []string) {
	argTypes := make([]string, 0, len(paramTypes)+len(freeTypes))
	argTypes = append(argTypes, paramTypeStrs...)
	for _, t := range freeTypes {
		argTypes = append(argTypes, e.getTypeFullName(t))
	}

	isConstArg := make([]bool, 0, len(paramTypes)+len(freeTypes))
	litValues := make([]string, 0, len(paramTypes)+len(freeTypes))
	for range paramTypes {
		isConstArg = append(isConstArg, false)
		litValues = append(litValues, "")
	}
	for i := range freeTypes {
		if i < len(freeIsConst) {
			isConstArg = append(isConstArg, freeIsConst[i])
			litValues = append(litValues, freeLitValues[i])
		} else {
			isConstArg = append(isConstArg, false)
			litValues = append(litValues, "")
		}
	}
	return argTypes, isConstArg, litValues
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
	prefix := "__p_"
	if isInvoke {
		prefix = "__i_"
	}
	return fmt.Sprintf("%s%d", prefix, len(e.items))
}

func (e *Extractor) buildExtractedItem(funcName string, argTypes []string, isInvoke, hasErr bool, curPkg *packages.Package, funcLit *ast.FuncLit,
	freeVars []*ast.Ident, freeTypes []types.Type, freeTypeStrs []string,
	paramNames []string, paramTypes []types.Type,
	isConstArg []bool, litValues []string, retType string, isContextArg []bool) extractedItem {

	item := extractedItem{
		FuncName:          funcName,
		ArgTypes:          argTypes,
		IsInvoke:          isInvoke,
		HasError:          hasErr,
		Pkg:               curPkg,
		PkgAlias:          e.collectPkgAlias(curPkg),
		IsClosure:         true,
		ClosureLit:        funcLit,
		FreeVars:          freeVars,
		FreeTypes:         freeTypes,
		FreeTypeStrings:   freeTypeStrs,
		ClosureParamNames: paramNames,
		ClosureParamTypes: paramTypes,
		IsConstArg:        isConstArg,
		ConstLitValues:    litValues,
		IsContextArg:      isContextArg,
	}
	if retType != "" {
		item.RetType = retType
	}
	return item
}

func (e *Extractor) handleInvoke(expr ast.Expr, curPkg *packages.Package) error {
	if funcLit, ok := expr.(*ast.FuncLit); ok {
		return e.handleFuncLit(funcLit, curPkg, true)
	}
	name, sig, realPkg, err := getFuncMeta(expr, curPkg, e.pkgMap)
	if err != nil {
		return err
	}
	alias := e.collectPkgAlias(realPkg)
	argTypes := make([]string, sig.Params().Len())
	isContextArg := make([]bool, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		typ := param.Type()
		argTypes[i] = e.getTypeFullName(typ)
		isContextArg[i] = isContextType(typ)
	}
	hasErr := sigHasError(sig)
	e.items = append(e.items, extractedItem{
		FuncName:     name,
		ArgTypes:     argTypes,
		IsInvoke:     true,
		Pkg:          realPkg,
		PkgAlias:     alias,
		HasError:     hasErr,
		IsContextArg: isContextArg,
	})
	return nil
}

// collectFreeVarsWithConst 收集闭包中的自由变量（变量和常量）
func (e *Extractor) collectFreeVarsWithConst(funcLit *ast.FuncLit, curPkg *packages.Package) ([]*ast.Ident, []types.Type, []string, []bool, []string, error) {
	declSet := e.collectDeclarations(funcLit)
	freeVars, freeTypes, freeTypeStrs, isConst, litValues := e.collectFreeVarsFromBody(funcLit.Body, curPkg, declSet)
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

// ========== 重构后的 collectDeclarations 及辅助 ==========

func (e *Extractor) collectDeclarations(funcLit *ast.FuncLit) map[string]bool {
	declSet := make(map[string]bool)
	// 收集参数声明
	e.collectParamDecls(funcLit, declSet)
	// 遍历函数体，收集赋值和声明语句中的定义
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

// collectParamDecls 收集函数字面量的参数名
func (e *Extractor) collectParamDecls(funcLit *ast.FuncLit, declSet map[string]bool) {
	if funcLit.Type.Params != nil {
		for _, field := range funcLit.Type.Params.List {
			for _, name := range field.Names {
				declSet[name.Name] = true
			}
		}
	}
}

// collectAssignDecls 从赋值语句中收集左侧标识符（变量声明）
func (e *Extractor) collectAssignDecls(assign *ast.AssignStmt, declSet map[string]bool) {
	for _, lhs := range assign.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Obj != nil && ident.Obj.Kind == ast.Var {
			declSet[ident.Name] = true
		}
	}
}

// collectGenDecls 从声明语句中收集值规格说明中的名称
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

// ========== 其他 Extractor 方法 ==========

func (e *Extractor) collectFreeVarsFromBody(body *ast.BlockStmt, curPkg *packages.Package, declSet map[string]bool) ([]*ast.Ident, []types.Type, []string, []bool, []string) {
	var freeVars []*ast.Ident
	var freeTypes []types.Type
	var freeTypeStrs []string
	var isConst []bool
	var litValues []string
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Obj != nil {
			return true
		}
		obj := curPkg.TypesInfo.ObjectOf(ident)
		if obj == nil {
			return true
		}
		if _, isDecl := declSet[ident.Name]; isDecl {
			return true
		}

		// 预声明常量直接跳过
		if cObj, ok := obj.(*types.Const); ok && cObj.Pkg() == nil {
			return true
		}

		// 用户自定义常量：只记录字面量，不加入freeVars
		if _, ok := obj.(*types.Const); ok {
			if seen[ident.Name] {
				return true
			}
			seen[ident.Name] = true
			// 常量不存入freeVars，跳过参数生成
			return true
		}

		// 仅变量类型才作为自由变量
		if _, ok := obj.(*types.Var); !ok {

			return true
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
	})

	return freeVars, freeTypes, freeTypeStrs, isConst, litValues
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

func (e *Extractor) handleSupply(expr ast.Expr, curPkg *packages.Package) error {
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := checkExportedVisibility(obj, curPkg.Types); err != nil {
			return err
		}
	}
	alias := e.collectPkgAlias(curPkg)
	typ := curPkg.TypesInfo.TypeOf(expr)
	if typ == nil {
		return fmt.Errorf("resolve supply type failed")
	}
	retType := e.getTypeFullName(typ)
	if _, dup := e.globalProviderMap[retType]; dup {
		return fmt.Errorf("duplicate supply for type %q", retType)
	}
	idx := len(e.items)
	e.items = append(e.items, extractedItem{
		IsSupply: true,
		RetType:  retType,
		Expr:     expr,
		Pkg:      curPkg,
		PkgAlias: alias,
	})
	e.globalProviderMap[retType] = idx
	return nil
}

func (e *Extractor) collectPkgAlias(pkg *packages.Package) string {
	pp := pkg.PkgPath
	if pp == "" || pkg.Module == nil {
		return ""
	}
	// 首先检查用户自定义别名
	if alias, ok := e.importAliasMap[pp]; ok {
		// 只有非主包才存储别名
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
	// 自动生成别名（原有逻辑）
	existing := make(map[string]bool)
	for _, a := range e.pkgAliasMap {
		existing[a] = true
	}
	for _, a := range e.importAliasMap {
		existing[a] = true
	}
	alias := uniqueAlias(pp, existing)
	// 只有非主包才存入 map
	if pp != e.mainPkgPath {
		e.pkgAliasMap[pp] = alias
	}
	if pp == e.mainPkgPath {
		return ""
	}
	return alias
}

// ----------------------------------------------------------------------------
// 循环检测与错误格式化
// ----------------------------------------------------------------------------

func (e *Extractor) findCycle(adj [][]int) ([]int, error) {
	n := len(adj)
	state := make([]int, n)
	parent := make([]int, n)
	var cycle []int
	var dfs func(int) bool
	dfs = func(u int) bool {
		state[u] = 1
		for _, v := range adj[u] {
			switch state[v] {
			case 0:
				parent[v] = u
				if dfs(v) {
					return true
				}
			case 1:
				cycle = []int{v}
				for cur := u; cur != v; cur = parent[cur] {
					cycle = append(cycle, cur)
				}
				return true
			}
		}
		state[u] = 2
		return false
	}
	for i := range n {
		if state[i] == 0 {
			parent[i] = -1
			if dfs(i) {
				return cycle, nil
			}
		}
	}
	return nil, fmt.Errorf("no cycle found")
}

func (e *Extractor) formatCycleError(cycle []int) error {
	var cycleDesc []string
	for _, idx := range cycle {
		cycleDesc = append(cycleDesc, e.describeItem(idx))
	}
	cycleInfo := strings.Join(cycleDesc, " -> ")
	return fmt.Errorf("circular dependency detected: %s", cycleInfo)
}

func (e *Extractor) describeItem(idx int) string {
	it := e.items[idx]
	var desc string
	if it.IsSupply {
		desc = fmt.Sprintf("Supply of type %q", it.RetType)
	} else if it.IsInvoke {
		funcName := fullFuncName(it.Pkg.PkgPath, it.FuncName)
		desc = fmt.Sprintf("Invoke %q", funcName)
	} else {
		funcName := fullFuncName(it.Pkg.PkgPath, it.FuncName)
		desc = fmt.Sprintf("Provider %q (returns %q)", funcName, it.RetType)
	}
	if len(it.ArgTypes) > 0 {
		desc += fmt.Sprintf(" depends on [%s]", strings.Join(it.ArgTypes, ", "))
	}
	return desc
}

// ----------------------------------------------------------------------------
// 构建最终节点（拆分后）
// ----------------------------------------------------------------------------

func (e *Extractor) buildFinalNodes() ([]Node, error) {
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
	if e.UnusedMode == UnusedModeError {
		if err := checkUnusedProviders(nodes); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (e *Extractor) buildDependencyGraph(items []extractedItem) ([][]int, []int, error) {
	n := len(items)
	adj := make([][]int, n)
	indeg := make([]int, n)
	for i, it := range items {
		if it.IsSupply {
			continue
		}
		for j, argType := range it.ArgTypes {
			if len(it.IsContextArg) > j && it.IsContextArg[j] {
				continue
			}
			if it.IsClosure && len(it.IsConstArg) > j && it.IsConstArg[j] {
				continue
			}
			providerIdx, ok := e.globalProviderMap[argType]
			if !ok {
				funcName := fullFuncName(it.Pkg.PkgPath, it.FuncName)
				return nil, nil, fmt.Errorf("no provider for type %q (required by %s)", argType, funcName)
			}
			adj[providerIdx] = append(adj[providerIdx], i)
			indeg[i]++
		}
	}
	return adj, indeg, nil
}

func (e *Extractor) computeOrder(adj [][]int, indeg []int) ([]int, error) {
	n := len(adj)
	indegCopy := make([]int, n)
	copy(indegCopy, indeg)

	order, err := topologicalSort(n, adj, indegCopy)
	if err != nil {
		cycle, cycleErr := e.findCycle(adj)
		if cycleErr != nil {
			return nil, fmt.Errorf("circular dependency (failed to locate cycle): %v", err)
		}
		return nil, e.formatCycleError(cycle)
	}
	return order, nil
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

func (e *Extractor) buildNodes(order []int, items []extractedItem, varNames []string) []Node {
	var final []Node
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

func (e *Extractor) resolveArgNames(it extractedItem, varNames []string) []string {
	argNames := make([]string, len(it.ArgTypes))
	for j, argType := range it.ArgTypes {
		if it.IsContextArg != nil && j < len(it.IsContextArg) && it.IsContextArg[j] {
			argNames[j] = "" // 占位，生成时替换为 "ctx"
			continue
		}
		provIdx := e.globalProviderMap[argType]
		argNames[j] = varNames[provIdx]
	}
	return argNames
}

func (e *Extractor) buildInvokeNode(it extractedItem, argNames []string) Node {
	node := Node{
		Func:      it.FuncName,
		FuncPkg:   it.PkgAlias,
		Args:      argNames,
		IsInvoke:  true,
		HasError:  it.HasError,
		IsClosure: it.IsClosure,

		PkgPath:      it.Pkg.PkgPath,
		IsContextArg: it.IsContextArg,
	}
	if it.IsClosure {
		closureDef, usedPkgs, err := e.generateClosureDef(&it)
		if err != nil {
			panic(err)
		}
		node.ClosureDef = closureDef
		node.UsedPkgs = usedPkgs
		// 也要传递 IsClosureParam, IsConstArg, ConstLitValues
		node.IsConstArg = it.IsConstArg
		node.ConstLitValues = it.ConstLitValues
	}
	return node
}

func (e *Extractor) buildSupplyNode(it extractedItem, name string) Node {
	var buf strings.Builder
	_ = printer.Fprint(&buf, it.Pkg.Fset, it.Expr)
	return Node{
		Name:     name,
		IsSupply: true,
		Value:    buf.String(),
		FuncPkg:  it.PkgAlias,
		PkgPath:  it.Pkg.PkgPath,
	}
}

func (e *Extractor) buildProviderNode(it extractedItem, argNames []string, name string) Node {
	var closureDef string
	var usedPkgs []string
	if it.IsClosure {
		var err error
		closureDef, usedPkgs, err = e.generateClosureDef(&it)
		if err != nil {
			panic(err)
		}
	}
	return Node{
		Name:      name,
		Func:      it.FuncName,
		FuncPkg:   it.PkgAlias,
		RetType:   it.RetType,
		Args:      argNames,
		HasError:  it.HasError,
		IsClosure: it.IsClosure,

		ClosureDef: closureDef,
		UsedPkgs:   usedPkgs,
		PkgPath:    it.Pkg.PkgPath,

		IsConstArg:     it.IsConstArg,
		ConstLitValues: it.ConstLitValues,
		IsContextArg:   it.IsContextArg,
	}
}
func buildCallArgs(node Node) []string {
	args := make([]string, len(node.Args))
	for i, arg := range node.Args {
		if len(node.IsContextArg) > i && node.IsContextArg[i] {
			args[i] = "ctx"
		} else {
			args[i] = arg
		}
	}
	return args
}

func (e *Extractor) replacePkgPathWithAlias(typeStr string) string {
	// 处理指针和切片等前缀
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

	// 去掉主包前缀
	if strings.HasPrefix(typeStr, e.mainPkgPath+".") {
		typeStr = typeStr[len(e.mainPkgPath)+1:]
	}

	// 其他包别名替换
	type pair struct {
		path  string
		alias string
	}
	var pairs []pair
	for path, alias := range e.pkgAliasMap {
		pairs = append(pairs, pair{path, alias})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return len(pairs[i].path) > len(pairs[j].path)
	})
	result := typeStr
	for _, p := range pairs {
		result = strings.ReplaceAll(result, p.path+".", p.alias+".")
	}
	return prefix.String() + result
}
func (e *Extractor) collectUsedPkgsFromBody(body *ast.BlockStmt, pkg *packages.Package, usedPkgs map[string]bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			obj := pkg.TypesInfo.ObjectOf(ident)
			if obj == nil {
				return true
			}
			if typeName, ok := obj.(*types.TypeName); ok {
				pkgPathObj := typeName.Pkg()
				if pkgPathObj == nil {
					return true // 内置类型，如 bool, error
				}
				pkgPath := pkgPathObj.Path()
				if pkgPath != "" && pkgPath != e.mainPkgPath {
					usedPkgs[pkgPath] = true
				}
			}
		}
		return true
	})
}

// typePkg 辅助方法（添加到 Extractor）
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
		// 简单处理：忽略 key 和 elem 的包（可根据需要完善）
		return nil
	default:
		return nil
	}
}

func (e *Extractor) generateClosureDef(it *extractedItem) (string, []string, error) {
	usedPkgs := make(map[string]bool)

	paramList, freeVarMap, err := e.buildParamListAndFreeVarMap(it, usedPkgs)
	if err != nil {
		return "", nil, err
	}
	paramStr := strings.Join(paramList, ", ")

	rewrittenBody := e.replaceFreeVarsInBody(it.ClosureLit.Body, freeVarMap)

	typeNameMap := e.collectTypeNameMap(rewrittenBody, it.Pkg)
	e.collectUsedPkgsFromBody(rewrittenBody, it.Pkg, usedPkgs)

	var bodyBuf bytes.Buffer
	if err := printer.Fprint(&bodyBuf, it.Pkg.Fset, rewrittenBody); err != nil {
		return "", nil, fmt.Errorf("printer print closure body failed: %w", err)
	}
	bodyStr := bodyBuf.String()

	bodyStr = e.applyTypeAliasReplacements(bodyStr, typeNameMap)

	retType := ""
	if it.RetType != "" {
		retType = e.replacePkgPathWithAlias(it.RetType)
	}
	def := e.buildClosureDefString(it.FuncName, paramStr, bodyStr, retType)

	var usedList []string
	for pkgPath := range usedPkgs {
		usedList = append(usedList, pkgPath)
	}
	return def, usedList, nil
}

// ---------- 辅助方法 for generateClosureDef ----------
func (e *Extractor) buildParamListAndFreeVarMap(it *extractedItem, usedPkgs map[string]bool) ([]string, map[string]string, error) {
	var paramList []string
	freeVarMap := make(map[string]string)
	paramIdx := 0

	// 闭包原始参数
	for i := 0; i < len(it.ClosureParamNames); i++ {
		name := it.ClosureParamNames[i]
		typStr := e.replacePkgPathWithAlias(it.ArgTypes[paramIdx])
		paramList = append(paramList, fmt.Sprintf("%s %s", name, typStr))
		if pkg := e.typePkg(it.ClosureParamTypes[i]); pkg != nil && pkg.Path() != e.mainPkgPath {
			usedPkgs[pkg.Path()] = true
		}
		paramIdx++
	}

	// 自由变量（只处理非常量）
	for i := 0; i < len(it.FreeVars); i++ {
		if i >= len(it.IsConstArg) || it.IsConstArg[i] {
			continue
		}
		paramName := fmt.Sprintf("p%d", i)
		typStr := e.replacePkgPathWithAlias(it.FreeTypeStrings[i])
		paramList = append(paramList, fmt.Sprintf("%s %s", paramName, typStr))
		freeVarMap[it.FreeVars[i].Name] = paramName
		if pkg := e.typePkg(it.FreeTypes[i]); pkg != nil && pkg.Path() != e.mainPkgPath {
			usedPkgs[pkg.Path()] = true
		}
	}
	return paramList, freeVarMap, nil
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

func (e *Extractor) collectTypeNameMap(body *ast.BlockStmt, pkg *packages.Package) map[string]string {
	typeNameMap := make(map[string]string)
	astutil.Apply(body,
		func(c *astutil.Cursor) bool {
			ident, ok := c.Node().(*ast.Ident)
			if !ok {
				return true
			}
			if sel, ok := c.Parent().(*ast.SelectorExpr); ok && sel.Sel == ident {
				return true // 已限定的类型名跳过
			}
			obj := pkg.TypesInfo.ObjectOf(ident)
			if typeName, ok := obj.(*types.TypeName); ok {
				pkgObj := typeName.Pkg()
				if pkgObj != nil && pkgObj.Path() != e.mainPkgPath {
					alias, found := e.pkgAliasMap[pkgObj.Path()]
					if !found {
						parts := strings.Split(pkgObj.Path(), "/")
						alias = parts[len(parts)-1]
					}
					typeNameMap[ident.Name] = alias + "." + ident.Name
				}
			}
			return true
		},
		nil,
	)
	return typeNameMap
}

func (e *Extractor) applyTypeAliasReplacements(bodyStr string, typeNameMap map[string]string) string {
	// 先替换独立的类型名
	for name, replacement := range typeNameMap {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		bodyStr = re.ReplaceAllString(bodyStr, replacement)
	}
	// 再替换完整的包路径
	return e.replacePkgPathWithAlias(bodyStr)
}

func (e *Extractor) buildClosureDefString(funcName, paramStr, bodyStr, retType string) string {
	if retType != "" {
		return fmt.Sprintf("func %s(%s) %s %s", funcName, paramStr, retType, bodyStr)
	}
	return fmt.Sprintf("func %s(%s) %s", funcName, paramStr, bodyStr)
}

// importInfo 用于收集导入信息
type importInfo struct {
	filePath string
	pkgPath  string
	alias    string
	isMain   bool
}

// collectAllImportInfos 遍历所有包，收集所有有效的导入别名信息，并按主包优先、文件路径排序
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
	// 排序：主包优先，同主包按文件路径排序（保持确定性）
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].isMain != infos[j].isMain {
			return infos[i].isMain // true（主包）排在前面
		}
		return infos[i].filePath < infos[j].filePath
	})
	return infos
}

// loadImportAliases 加载用户自定义的导入别名，主包别名优先
func (e *Extractor) loadImportAliases() {
	infos := e.collectAllImportInfos()
	for _, info := range infos {
		if _, exists := e.importAliasMap[info.pkgPath]; !exists {
			e.importAliasMap[info.pkgPath] = info.alias
		}
	}
}

// ----------------------------------------------------------------------------
// 拓扑排序、未使用检查
// ----------------------------------------------------------------------------

func topologicalSort(n int, adj [][]int, indeg []int) ([]int, error) {
	queue := []int{}
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

func checkUnusedProviders(nodes []Node) error {
	refCount := make(map[string]int)
	for _, node := range nodes {
		for _, arg := range node.Args {
			refCount[arg]++
		}
	}
	for _, node := range nodes {
		if node.IsInvoke {
			continue
		}
		if node.HasError {
			continue
		}
		if refCount[node.Name] == 0 {
			funcDesc := fullFuncName(node.FuncPkg, node.Func)
			return fmt.Errorf("unused provider: %s (returns %s)", funcDesc, node.RetType)
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// 代码生成（拆分）
// ----------------------------------------------------------------------------

func generateCode(nodes []Node, pkgName, originFuncName, diPath, diAlias, mainPkgPath string, pkgAliasMap map[string]string, unusedMode UnusedMode) string {
	mainPkg := pkgName
	if mainPkg == "" {
		mainPkg = "main"
	}
	buf := &bytes.Buffer{}

	writeHeader(buf, mainPkg)

	// 构建 usedPkgSet
	usedPkgSet := make(map[string]bool)
	usedPkgSet["context"] = true
	usedPkgSet[diPath] = true

	for _, node := range nodes {
		if node.PkgPath != "" && node.PkgPath != mainPkgPath {
			usedPkgSet[node.PkgPath] = true
		}
		for _, pkgPath := range node.UsedPkgs {
			usedPkgSet[pkgPath] = true
		}
	}

	var usedPkgs []string
	for pkgPath := range usedPkgSet {
		usedPkgs = append(usedPkgs, pkgPath)
	}

	writeImports(buf, mainPkgPath, pkgAliasMap, usedPkgs, diPath, diAlias)
	writeClosureDefs(buf, nodes)
	writeMainFunc(buf, nodes, originFuncName, diAlias, unusedMode)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		debugf("warning: failed to format generated code: %v; writing unformatted", err)
		return buf.String()
	}
	return string(formatted)
}

func writeHeader(buf *bytes.Buffer, pkgName string) {
	fmt.Fprintf(buf, "// Code generated by %s; DO NOT EDIT.\n\n", tagBuild)
	fmt.Fprintf(buf, "//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/%s\n", tagBuild)
	fmt.Fprintf(buf, "//go:build !%s\n", tagBuild)
	fmt.Fprintf(buf, "// +build !%s\n\n", tagBuild)
	fmt.Fprintf(buf, "package %s\n\n", pkgName)
}

func writeImports(buf *bytes.Buffer, mainPkgPath string, pkgAliasMap map[string]string, usedPkgs []string, diPath, diAlias string) {
	importMap := make(map[string]string)
	for _, pkgPath := range usedPkgs {
		if pkgPath == mainPkgPath || pkgPath == "" {
			continue
		}
		alias, ok := pkgAliasMap[pkgPath]
		if !ok {
			parts := strings.Split(pkgPath, "/")
			alias = parts[len(parts)-1]
		}
		importMap[pkgPath] = alias
	}
	var paths []string
	for path := range importMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	buf.WriteString("import (\n")
	for _, path := range paths {
		alias := importMap[path]
		// 获取默认包名（路径最后一段）
		parts := strings.Split(path, "/")
		defaultName := parts[len(parts)-1]
		if alias == defaultName {
			// 省略别名，只写路径
			fmt.Fprintf(buf, "\t%q\n", path)
		} else {
			fmt.Fprintf(buf, "\t%s %q\n", alias, path)
		}
	}
	buf.WriteString(")\n\n")
}

func writeClosureDefs(buf *bytes.Buffer, nodes []Node) {
	for _, node := range nodes {
		if node.IsClosure && node.ClosureDef != "" {
			fmt.Fprintf(buf, "%s\n", node.ClosureDef)
		}
	}
	if len(nodes) > 0 {
		buf.WriteString("\n")
	}
}

func writeMainFunc(buf *bytes.Buffer, nodes []Node, originFuncName, diAlias string, unusedMode UnusedMode) {
	refCount := make(map[string]int)
	for _, node := range nodes {
		for _, arg := range node.Args {
			refCount[arg]++
		}
	}

	fmt.Fprintf(buf, "func %s() *%s.App {\n", originFuncName, diAlias)
	writeProviders(buf, nodes, refCount, unusedMode)
	fmt.Fprintf(buf, "\treturn %s.New(func(ctx context.Context) error {\n", diAlias)
	writeInvokes(buf, nodes)
	buf.WriteString("\t\treturn nil\n\t})\n}\n\n")
	// fmt.Fprintf(buf, "func %s() *%s.App {\n\treturn __%s()\n}\n", originFuncName, diAlias, originFuncName)
}

func writeProviders(buf *bytes.Buffer, nodes []Node, refCount map[string]int, unusedMode UnusedMode) {
	for _, node := range nodes {
		if node.IsInvoke {
			continue
		}
		if !node.HasError && refCount[node.Name] == 0 {
			if handleUnusedProvider(buf, node, unusedMode) {
				continue
			}
		}
		writeProviderStatement(buf, node)
	}
}

func writeInvokes(buf *bytes.Buffer, nodes []Node) {
	for _, node := range nodes {
		if !node.IsInvoke {
			continue
		}
		if node.IsClosure {
			args := buildCallArgs(node)
			argsStr := strings.Join(args, ", ")
			if node.HasError {
				fmt.Fprintf(buf, "\t\tif err := %s(%s); err != nil { return err }\n", node.Func, argsStr)
			} else {
				fmt.Fprintf(buf, "\t\t%s(%s)\n", node.Func, argsStr)
			}
			continue
		}
		full := fullFuncName(node.FuncPkg, node.Func)
		debugf("[DEBUG generateCode invoke] full=%q, FuncPkg=%q, Func=%q, args=%v", full, node.FuncPkg, node.Func, node.Args)
		args := buildCallArgs(node) // 替换原有 args
		argsStr := strings.Join(args, ", ")
		if node.HasError {
			if argsStr == "" {
				fmt.Fprintf(buf, "\t\tif err := %s(); err != nil { return err }\n", full)
			} else {
				fmt.Fprintf(buf, "\t\tif err := %s(%s); err != nil { return err }\n", full, argsStr)
			}
		} else {
			if argsStr == "" {
				fmt.Fprintf(buf, "\t\t%s()\n", full)
			} else {
				fmt.Fprintf(buf, "\t\t%s(%s)\n", full, argsStr)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// 生成辅助语句
// ----------------------------------------------------------------------------

func handleUnusedProvider(buf *bytes.Buffer, node Node, unusedMode UnusedMode) bool {
	switch unusedMode {
	case UnusedModeDrop:
		return true
	case UnusedModeIgnore:
		if node.IsSupply {
			expr := node.Value
			if node.FuncPkg != "" && !strings.HasPrefix(expr, node.FuncPkg+".") {
				expr = node.FuncPkg + "." + expr
			}
			fmt.Fprintf(buf, "\t_ = %s\n", expr)
		} else if node.IsClosure {
			argsStr := strings.Join(node.Args, ", ")
			fmt.Fprintf(buf, "\t_ = %s(%s)\n", node.Func, argsStr)
		} else {
			full := fullFuncName(node.FuncPkg, node.Func)
			args := strings.Join(node.Args, ", ")
			fmt.Fprintf(buf, "\t_ = %s(%s)\n", full, args)
		}
		return true
	default:
		return false
	}
}

func writeProviderStatement(buf *bytes.Buffer, node Node) {
	if node.IsSupply {
		expr := node.Value
		if node.FuncPkg != "" && !strings.HasPrefix(expr, node.FuncPkg+".") {
			expr = node.FuncPkg + "." + expr
		}
		fmt.Fprintf(buf, "\t%s := %s\n", node.Name, expr)
		return
	}
	if node.IsClosure {
		args := buildCallArgs(node) // 替换原有 args 构造
		argsStr := strings.Join(args, ", ")
		fmt.Fprintf(buf, "\t%s := %s(%s)\n", node.Name, node.Func, argsStr)
		return
	}
	full := fullFuncName(node.FuncPkg, node.Func)
	args := buildCallArgs(node)
	argsStr := strings.Join(args, ", ")
	if node.HasError {
		fmt.Fprintf(buf, "\t%s, err := %s(%s)\n\tif err != nil { panic(err) }\n", node.Name, full, argsStr)
	} else {
		fmt.Fprintf(buf, "\t%s := %s(%s)\n", node.Name, full, argsStr)
	}
}

// ----------------------------------------------------------------------------
// 调试日志
// ----------------------------------------------------------------------------

func debugf(format string, args ...any) {
	if debugEnabled {
		fmt.Printf("[digen]"+format+"\n", args...)
	}
}

// ----------------------------------------------------------------------------
// 重构后的 run 函数及辅助
// ----------------------------------------------------------------------------

func main() {
	flag.BoolVar(&debugEnabled, "debug", false, "enable debug logging")
	flag.Parse()

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run 作为主控流程，调用各个子步骤
func run() error {
	// 1. 解析模式
	unusedMode, err := parseUnusedMode()
	if err != nil {
		return err
	}

	// 2. 加载并校验包
	pkg, pkgMap, err := loadAndValidatePackages()
	if err != nil {
		return err
	}

	// 4. 查找目标注入函数
	target, err := findInjectorFunctions(pkg)
	if err != nil {
		return fmt.Errorf("scan target failed: %v", err)
	}

	// 5. 提取 DI 配置并构建节点
	nodes, pkgAliasMap, err := extractAndBuildNodes(pkg, target, pkgMap, unusedMode)
	if err != nil {
		return fmt.Errorf("extract and build nodes failed: %v", err)
	}

	// 6. 生成代码并写入文件
	if err := writeGeneratedCode(pkg, target, nodes, pkgAliasMap, unusedMode); err != nil {
		return err
	}

	debugf("generate success: %s", *outputFile)
	return nil
}

// parseUnusedMode 解析命令行参数中的 unused 模式
func parseUnusedMode() (UnusedMode, error) {
	switch *unusedModeStr {
	case "ignore":
		return UnusedModeIgnore, nil
	case "drop":
		return UnusedModeDrop, nil
	default:
		return UnusedModeError, nil
	}
}

// loadAndValidatePackages 加载当前目录下的包并检查错误
func loadAndValidatePackages() (*packages.Package, map[string]*packages.Package, error) {
	cfg := &packages.Config{
		Mode:       packages.NeedSyntax | packages.NeedTypes | packages.NeedName | packages.NeedModule | packages.NeedFiles | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests:      false,
		BuildFlags: []string{fmt.Sprintf("-tags=%s", tagBuild)},
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("packages.Load failed: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no packages loaded")
	}

	// 构建包映射
	pkgMap := collectAllPackages(pkgs)

	// 收集所有包的错误信息
	var errs []string
	for _, p := range pkgMap {
		if len(p.Errors) > 0 {
			for _, e := range p.Errors {
				debugf("package error in %s: %v", p.PkgPath, e)
				errs = append(errs, fmt.Sprintf("package %s: %v", p.PkgPath, e))
			}
		}
	}
	if len(errs) > 0 {
		return nil, nil, fmt.Errorf("compilation errors found in packages:\n%s", strings.Join(errs, "\n"))
	}

	// 主包（第一个）
	mainPkg := pkgs[0]
	if len(mainPkg.Errors) > 0 {
		var mainErrs []string
		for _, e := range mainPkg.Errors {
			debugf("package error: %v", e)
			mainErrs = append(mainErrs, e.Error())
		}
		return nil, nil, fmt.Errorf("main package contains errors: %s", strings.Join(mainErrs, "; "))
	}
	return mainPkg, pkgMap, nil
}

// extractAndBuildNodes 创建 Extractor，提取选项，构建最终节点
func extractAndBuildNodes(pkg *packages.Package, target *GenTarget, pkgMap map[string]*packages.Package, unusedMode UnusedMode) ([]Node, map[string]string, error) {
	entryFunc := target.Node
	buildCall := findBuildCall(entryFunc, pkg.TypesInfo)
	if buildCall == nil {
		return nil, nil, fmt.Errorf("no dig.Build call found")
	}

	extractor := &Extractor{
		pkgMap:            pkgMap,
		mainPkgPath:       pkg.PkgPath,
		items:             []extractedItem{},
		globalProviderMap: make(map[string]int),
		pkgAliasMap:       make(map[string]string),
		UnusedMode:        unusedMode,
		importAliasMap:    make(map[string]string),
	}
	extractor.loadImportAliases()
	// 提取所有选项
	for _, arg := range buildCall.Args {
		if err := extractor.extractOptions(arg, pkg, pkg); err != nil {
			return nil, nil, err
		}
	}

	// 构建最终节点
	nodes, err := extractor.buildFinalNodes()
	if err != nil {
		return nil, nil, err
	}
	return nodes, extractor.pkgAliasMap, nil
}

// writeGeneratedCode 生成代码并写入文件
func writeGeneratedCode(pkg *packages.Package, target *GenTarget, nodes []Node, pkgAliasMap map[string]string, unusedMode UnusedMode) error {
	// 从主包的导入声明中查找 dig 包的别名
	diAlias := ""
	for _, f := range pkg.Syntax {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == diPkgPath {
				if imp.Name != nil {
					diAlias = imp.Name.Name
				} else {
					parts := strings.Split(path, "/")
					diAlias = parts[len(parts)-1] // 默认包名
				}
				break
			}
		}
		if diAlias != "" {
			break
		}
	}
	if diAlias == "" {
		diAlias = "dig" // fallback
	}
	// 确保 pkgAliasMap 中包含该映射，供 generateCode 内部使用
	if _, ok := pkgAliasMap[diPkgPath]; !ok {
		pkgAliasMap[diPkgPath] = diAlias
	}

	code := generateCode(
		nodes,
		pkg.Name,
		target.FuncName,
		diPkgPath,
		diAlias,
		pkg.PkgPath,
		pkgAliasMap,
		unusedMode,
	)
	if err := os.WriteFile(*outputFile, []byte(code), 0644); err != nil {
		return err
	}
	return nil
}
