package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/alias"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"reflect"
	"strings"
)

func (e *Extractor) buildParamListAndFreeVarMap(it *extractedItem, usedPkgs map[string]bool) ([]string, map[string]string, map[string]string) {
	var paramList []string
	freeVarMap := make(map[string]string)
	constMap := make(map[string]string)

	// ShadowGuard 确保自由变量参数名不遮蔽包别名或闭包参数名
	sg := alias.NewShadowGuard(
		e.aliasManager.GetImportAliasMap(),
		e.aliasManager.GetPkgAliasMap(),
		e.aliasManager.GetPkgNameMap(),
	)

	// 闭包参数
	for _, arg := range it.ClosureParams {
		typStr := e.replacePkgPathWithAlias(arg.TypeString)
		paramList = append(paramList, arg.Name+" "+typStr)
		e.addPkgToUsed(arg.Type, usedPkgs)
		sg.Reserve(arg.Name) // 闭包参数名加入保留集，防止自由变量参数名与之冲突
	}

	// 自由变量（从 Params 中取闭包参数之后的部分）
	startIdx := len(it.ClosureParams)
	for i := startIdx; i < len(it.Params); i++ {
		arg := it.Params[i]
		if arg.IsConst {
			constMap[arg.Name] = arg.ConstValue
			continue
		}
		// 使用 ShadowGuard 选择不冲突的参数名（兼顾闭包参数名和包别名）
		// 保留既有 _fv 后缀约定：仅在发生冲突时使用
		paramName := arg.Name
		if sg.Reserved()[paramName] {
			paramName = sg.SafeName(arg.Name + "_fv")
		}
		sg.Reserve(paramName) // 防止后续自由变量重名

		typStr := e.replacePkgPathWithAlias(arg.TypeString)
		paramList = append(paramList, paramName+" "+typStr)
		freeVarMap[arg.Name] = paramName
		e.addPkgToUsed(arg.Type, usedPkgs)
	}

	return paramList, freeVarMap, constMap
}

func (e *Extractor) replaceFreeVarsInBody(body *ast.BlockStmt, freeVarMap map[string]string, constMap map[string]string) *ast.BlockStmt {
	newNode := astutil.Apply(body,
		func(c *astutil.Cursor) bool {
			if ident, ok := c.Node().(*ast.Ident); ok {
				// Phase 1: Replace constant references with literal values
				if constVal, ok := constMap[ident.Name]; ok {
					expr, err := strToExpr(constVal)
					if err == nil {
						c.Replace(expr)
						return false
					}
				}
				// Phase 2: Replace free variable references with parameter names
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

func strToExpr(s string) (ast.Expr, error) {
	expr, err := parser.ParseExpr(s)
	if err != nil {
		return nil, err
	}
	return expr, nil
}

// collectTypeNameAndUsedPkgs walks the closure body and records, for every
// cross-package type or package-level function reference, the source position ->
// aliased selector (e.g. "helper.Bootstrap") that must be substituted when the
// closure is moved into the main package.
//
// The result is keyed by token.Pos (the position of the original *ast.Ident) rather
// than by name, so the later rewrite can be done precisely on an AST clone without
// touching string literals, comments, or same-named local variables. The shared,
// type-checked source AST (loaded once per external package and shared across main
// packages) is only read here via pkg.TypesInfo and never mutated.
func (e *Extractor) collectTypeNameAndUsedPkgs(body *ast.BlockStmt, pkg *packages.Package, usedPkgs map[string]bool) map[token.Pos]string {
	typeNameMap := make(map[token.Pos]string)
	// qualifiedSel marks the .Sel identifier of a SelectorExpr (e.g. the "Config"
	// in "config.Config"). Such identifiers are already package-qualified and must
	// NOT be rewritten, otherwise we would produce "config.alias.Config".
	qualifiedSel := make(map[*ast.Ident]bool)
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			qualifiedSel[sel.Sel] = true
			return true
		}
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if qualifiedSel[ident] {
			return true
		}
		obj := pkg.TypesInfo.ObjectOf(ident)
		if obj == nil {
			return true
		}

		// 处理类型名（如 helper 包内裸写 Config）
		if typeName, ok := obj.(*types.TypeName); ok {
			pkgObj := typeName.Pkg()
			if pkgObj != nil && pkgObj.Path() != e.mainPkgPath {
				alias := e.aliasManager.EnsureAlias(pkgObj.Path())
				if alias != "" {
					typeNameMap[ident.Pos()] = alias + "." + ident.Name
					usedPkgs[pkgObj.Path()] = true
				}
			}
			return true
		}

		// 处理跨包函数名（如 setup.BootstrapStore）。
		// 闭包从非主包提取到主包时，裸函数标识符需补包前缀。此处仅记录其源位置与
		// 目标别名选择器，由后续 applyTypeAliasReplacements 在 AST 克隆上精确改写，
		// 覆盖调用 Func()、&Func、cb := Func 等所有引用场景。
		// 注意：不可在原地（共享 AST）构造 SelectorExpr 改写，因为 go/packages 仅加载
		// 一次 setup 包，其闭包体 AST 会被多个 main 包共享；原地变异会使后续包的
		// TypesInfo.ObjectOf 失效，导致漏加包导入。*types.Func 必为包级函数，无局部变量混淆风险。
		if fn, ok := obj.(*types.Func); ok {
			fnPkg := fn.Pkg()
			if fnPkg != nil && fnPkg.Path() != e.mainPkgPath {
				alias := e.aliasManager.EnsureAlias(fnPkg.Path())
				if alias != "" {
					typeNameMap[ident.Pos()] = alias + "." + ident.Name
					usedPkgs[fnPkg.Path()] = true
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

		// 处理跨包导出变量/常量（如 mcp.TransportStdio）。
		// 这些符号在闭包体内以裸标识符出现（Var/Const 不像 Func/TypeName 走
		// selector 路径），当其定义包非主包时，闭包被提升到主包后裸写会触发
		// "undefined: <Name>"。此处与 TypeName/Func 同理记录其源位置与目标
		// 别名选择器，由 applyTypeAliasReplacements 在 AST 克隆上精确改写为
		// <alias>.<Name>。注意：同包导出的裸标识符无需限定（pkgPath == mainPkgPath，
		// 跳过）；已带限定符的 SelectorExpr.Sel 已被 qualifiedSel 排除。
		// 关键闸门：仅覆盖**包级**符号（obj.Parent() == obj.Pkg().Scope()）。
		// *types.Var / *types.Const 既可是包级也可是局部（闭包参数、闭包内
		// 局部变量/常量），局部符号必须交由 free-var 通道处理，绝不在此限定，
		// 否则会出现 "supply_param_helper.c" 这类把闭包参数误限定为包符号的
		// 回归。
		if v, ok := obj.(*types.Var); ok {
			if v.Parent() == v.Pkg().Scope() {
				vPkg := v.Pkg()
				if vPkg != nil && vPkg.Path() != e.mainPkgPath {
					alias := e.aliasManager.EnsureAlias(vPkg.Path())
					if alias != "" {
						typeNameMap[ident.Pos()] = alias + "." + ident.Name
						usedPkgs[vPkg.Path()] = true
					}
				}
			}
			return true
		}
		if c, ok := obj.(*types.Const); ok {
			if c.Parent() == c.Pkg().Scope() {
				cPkg := c.Pkg()
				if cPkg != nil && cPkg.Path() != e.mainPkgPath {
					alias := e.aliasManager.EnsureAlias(cPkg.Path())
					if alias != "" {
						typeNameMap[ident.Pos()] = alias + "." + ident.Name
						usedPkgs[cPkg.Path()] = true
					}
				}
			}
			return true
		}

		return true
	})
	return typeNameMap
}

// applyTypeAliasReplacements rewrites cross-package type/func identifiers in the
// closure body to use their import aliases. It resolves the rewrite plan to exact
// AST node positions (see collectTypeNameAndUsedPkgs) and applies it on a clone of
// the body, so the shared, type-checked source AST is never mutated in place.
//
// Unlike the previous regex-based approach (replaceTypeNames), this cannot corrupt
// string literals or comments, and cannot accidentally rewrite a same-named local
// variable, because it operates only on identifiers that pkg.TypesInfo resolves to
// the target object.
// deepCloneAST returns a deep copy of an ast.Node subtree. It stands in for the
// standard library's ast.Clone (unavailable in this toolchain) so that alias
// rewrites can be applied to an isolated copy instead of the shared, type-checked
// source AST.
func deepCloneAST(n ast.Node) ast.Node {
	if n == nil {
		return nil
	}
	seen := make(map[uintptr]reflect.Value)
	return cloneValue(reflect.ValueOf(n), seen).Interface().(ast.Node)
}

// cloneValue is a reflection-based deep copy. The seen map breaks cycles that can
// appear via *ast.Object back-references, and also dedupes shared sub-nodes.
func cloneValue(v reflect.Value, seen map[uintptr]reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		addr := v.Pointer()
		if c, ok := seen[addr]; ok {
			return c
		}
		out := reflect.New(v.Type().Elem())
		seen[addr] = out
		cloned := cloneValue(v.Elem(), seen)
		if cloned.IsValid() {
			out.Elem().Set(cloned)
		}
		return out
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		return cloneValue(v.Elem(), seen)
	case reflect.Struct:
		t := v.Type()
		out := reflect.New(t).Elem()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath != "" {
				continue
			}
			cf := cloneValue(v.Field(i), seen)
			if cf.IsValid() {
				out.Field(i).Set(cf)
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		for i := 0; i < v.Len(); i++ {
			cf := cloneValue(v.Index(i), seen)
			if cf.IsValid() {
				out.Index(i).Set(cf)
			}
		}
		return out
	default:
		if !v.IsValid() {
			return reflect.Value{}
		}
		return v
	}
}

func (e *Extractor) applyTypeAliasReplacements(body *ast.BlockStmt, posRepl map[token.Pos]string) *ast.BlockStmt {
	if len(posRepl) == 0 {
		return body
	}
	cloned := deepCloneAST(body)
	clone, ok := cloned.(*ast.BlockStmt)
	if !ok {
		return body
	}
	astutil.Apply(clone, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok {
			return true
		}
		repl, ok := posRepl[ident.Pos()]
		if !ok {
			return true
		}
		if dot := strings.LastIndex(repl, "."); dot > 0 {
			c.Replace(&ast.SelectorExpr{
				X:   ast.NewIdent(repl[:dot]),
				Sel: ast.NewIdent(repl[dot+1:]),
			})
		}
		return true
	}, nil)
	return clone
}

func supplyValueNeedsQualification(expr ast.Expr, info *types.Info) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		// Non-bare shape (call, composite literal, selector, ...): keep the
		// historical qualification behavior untouched.
		return true
	}
	obj := info.ObjectOf(ident)
	if obj == nil {
		// Unresolved identifier: preserve historical behavior (qualify).
		return true
	}
	if obj.Pkg() == nil {
		// Builtin or universe-scoped object: preserve historical behavior.
		return true
	}
	// Bare identifier: qualify ONLY when it names a package-level symbol.
	// Free variables (parameters and locals of the inlined module) live in a
	// nested scope and must be referenced verbatim.
	return obj.Parent() == obj.Pkg().Scope()
}

// isSupplyValueFreeVar reports whether the Supply value expression is a bare
// identifier that names a function parameter or a local variable of the inlined
// module — i.e. a free variable captured by the generation target's scope
// rather than a package-level symbol.
//
// Free-variable supplies must NOT be package-qualified (that yields
// `undefined: <pkg>.<param>`) and must NOT pull an import for their defining
// package: the inlined statement (e.g. `_ = cfg`) references only the target
// scope.
func isSupplyValueFreeVar(expr ast.Expr, info *types.Info) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	obj := info.ObjectOf(ident)
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	// Package-level objects live in the package scope; parameters and locals
	// live in a nested scope.
	return obj.Parent() != obj.Pkg().Scope()
}

func (e *Extractor) assignVarNames(order []int, items []extractedItem) []string {
	n := len(items)
	varNames := make([]string, n)
	// ShadowGuard 作为安全网，防止 dvN 与包别名或内建标识符冲突。
	// dv 前缀（digen variable）与所有别名策略生成的格式不重叠
	// （SimpleAliasStrategy 生成 <base>/base2/base3，ContextualAliasStrategy 生成 <segment> 或 <seg1>_<seg2>，
	// ObfuscatedAliasStrategy 生成单字母+数字，NumericAliasStrategy 生成 _N）。
	sg := alias.NewShadowGuard(
		e.aliasManager.GetImportAliasMap(),
		e.aliasManager.GetPkgAliasMap(),
		e.aliasManager.GetPkgNameMap(),
	)
	vIdx := 0
	for _, i := range order {
		if !items[i].IsInvoke {
			name := sg.SafeName(fmt.Sprintf("dv%d", vIdx))
			// 把已分配的名字加入保留集，防止后续 SafeName 回退到同名
			sg.Reserve(name)
			varNames[i] = name
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
				return fmt.Errorf("at %s: cannot resolve type of parameter %s", pos, name.Name)
			}
			retType := extractor.getTypeFullName(typ)
			if seenTypes[retType] {
				return fmt.Errorf("at %s: duplicate parameter type %q (parameter %s)", pos, retType, name.Name)
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

func validateReturnType(fnDecl *ast.FuncDecl, info *types.Info, fset *token.FileSet) error {
	pos := fset.Position(fnDecl.Pos())
	if fnDecl.Type.Results == nil || len(fnDecl.Type.Results.List) == 0 {
		return fmt.Errorf("at %s: function %q: must have a return value of type func(context.Context) error", pos, fnDecl.Name.Name)
	}
	if len(fnDecl.Type.Results.List) > 1 {
		return fmt.Errorf("at %s: function %q: only a single return value allowed, expected func(context.Context) error", pos, fnDecl.Name.Name)
	}
	resField := fnDecl.Type.Results.List[0]
	if len(resField.Names) > 0 {
		return fmt.Errorf("at %s: function %q: named return value is not allowed, expected func(context.Context) error", pos, fnDecl.Name.Name)
	}
	retType := info.TypeOf(resField.Type)
	if retType == nil {
		return fmt.Errorf("at %s: function %q: failed to resolve return type", pos, fnDecl.Name.Name)
	}
	if !isContextFunc(retType) {
		return fmt.Errorf("at %s: function %q: invalid return type %q, expected func(context.Context) error", pos, fnDecl.Name.Name, retType.String())
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

func ValidateReturnType(fnDecl *ast.FuncDecl, info *types.Info, fset *token.FileSet) error {
	return validateReturnType(fnDecl, info, fset)
}

func FindBuildCall(fn *ast.FuncDecl, info *types.Info) *ast.CallExpr {
	if fn.Body == nil {
		return nil
	}
	return findDigCallInBlock(fn.Body, info, "Build")
}

func (e *Extractor) PkgAliasMap() map[string]string {
	return e.aliasManager.GetPkgAliasMap()
}
