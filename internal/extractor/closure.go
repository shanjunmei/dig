package extractor

import (
	"bytes"
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/alias"
	"github.com/shanjunmei/dig/pkg/functional"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
)

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
				err = fmt.Errorf("at %s: cannot capture local variable %q defined in InitApp scope; pass it as a parameter to the function (preferred) or move it to package level", curPkg.Fset.Position(ident.Pos()), ident.Name)
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
				err = fmt.Errorf("at %s: cannot capture local constant %q defined in InitApp scope; pass it as a parameter to the function (preferred) or move it to package level", curPkg.Fset.Position(ident.Pos()), ident.Name)
				return false
			}
			constVal := e.extractConstLiteral(o)
			if seen[ident.Name] {
				return true
			}
			seen[ident.Name] = true
			freeVars = append(freeVars, ident)
			freeTypes = append(freeTypes, obj.Type())
			freeTypeStrs = append(freeTypeStrs, e.getTypeFullName(obj.Type()))
			isConst = append(isConst, true)
			litValues = append(litValues, constVal)
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

func (e *Extractor) collectFreeVarsWithConst(funcLit *ast.FuncLit, curPkg *packages.Package) ([]*ast.Ident, []types.Type, []string, []bool, []string, error) {
	declSet := e.collectDeclarations(funcLit)
	freeVars, freeTypes, freeTypeStrs, isConst, litValues, err := e.collectFreeVarsFromBody(funcLit.Body, curPkg, declSet)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	for _, ident := range freeVars {
		obj := curPkg.TypesInfo.ObjectOf(ident)
		if obj != nil && isContextType(obj.Type()) {
			return nil, nil, nil, nil, nil, fmt.Errorf("at %s: cannot capture context variable %q as free variable; please pass context as a function parameter", curPkg.Fset.Position(ident.Pos()), ident.Name)
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
		return "", fmt.Errorf("at %s: anonymous provide function has no return", curPkg.Fset.Position(funcLit.Pos()))
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

func (e *Extractor) handleFuncLit(funcLit *ast.FuncLit, curPkg *packages.Package, isInvoke bool) error {
	// 1. 验证签名
	sig, err := e.validateClosureSignature(funcLit, curPkg, isInvoke)
	if err != nil {
		return err
	}

	// 2. 检查闭包体内的方法调用对生成目标包的可见性
	if err := e.checkMethodVisibilityInClosure(funcLit.Body, curPkg); err != nil {
		return err
	}

	// 2b. 检查闭包体内的裸函数/类型调用对生成目标包的可见性
	// （例如同包未导出函数 buildAuditAuthorizer 被提升到主包后变成 pkg.buildAuditAuthorizer）
	if err := e.checkFunctionVisibilityInClosure(funcLit.Body, curPkg); err != nil {
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
	// 4a. 提取命名返回值（必须在重复检查之前完成，以构建正确的键）
	instanceName := e.extractNamedReturn(sig)
	if retType != "" {
		// 构建与 registerClosureProvider 一致的键格式：默认实例用 retType，命名实例用 retType:instanceName
		key := retType
		if instanceName != "" {
			key = retType + ":" + instanceName
		}
		if _, dup := e.globalProviderMap[key]; dup {
			pos := curPkg.Fset.Position(funcLit.Pos())
			if instanceName != "" {
				return fmt.Errorf("at %s: duplicate binding for %s with name %q", pos, retType, instanceName)
			}
			return fmt.Errorf("at %s: duplicate provide for type %q", pos, retType)
		}
	}

	// 5. 构建 extractedItem
	funcName := e.generateFuncName(isInvoke)
	hasErr := sigHasError(sig)
	item := e.newExtractedItem(funcName, curPkg, e.aliasManager.CollectPkgAlias(curPkg), hasErr)
	item.IsInvoke = isInvoke
	item.IsClosure = true
	item.ClosureLit = funcLit
	item.FreeVars = freeVars
	item.FreeTypes = freeTypes
	item.FreeTypeStrings = freeTypeStrs
	item.Params = params
	item.ClosureParams = closureParams

	// Providers cannot take a context.Context parameter (resolved before ctx exists).
	if err := e.checkProviderContextParams(item, curPkg.Fset.Position(funcLit.Pos())); err != nil {
		return err
	}

	// Phase 3: Analyze inlinability (IIFE inlining) — gated by the -inline flag
	// (default off). This ONLY controls the "inline as IIFE" optimization; it
	// does NOT gate identity-closure collapse (Phase 4), which is applied
	// unconditionally below regardless of -inline.
	if e.cfg.InlineClosures {
		// Build isConst slice from params (after closure params, these are free vars)
		// Invariant: len(params) == len(closureParams) + len(freeVars) (guaranteed by buildClosureArgumentLists)
		freeVarIsConst := make([]bool, len(freeVars))
		startIdx := len(closureParams)
		for i := range freeVars {
			freeVarIsConst[i] = params[startIdx+i].IsConst
		}
		item.ShouldInline = analyzeClosureInlinability(funcLit, freeVars, freeVarIsConst)
	}

	// Phase 4: Analyze identity closure — ALWAYS applied (not gated by -inline).
	// Identity closures are literal-equivalent type conversions (T(p), &p, *p,
	// U(p)) with zero runtime semantic change, so collapsing them unconditionally
	// is safe. Takes priority over regular IIFE inlining when both apply.
	if retTypeExpr, opType := analyzeIdentityClosure(funcLit, freeVars); retTypeExpr != nil {
		typeObj := curPkg.TypesInfo.TypeOf(retTypeExpr)
		// 先确保返回类型所在包的别名已生成（buildClosureDef 中的 EnsureAlias 此时未执行），
		// 否则 replacePkgPathWithAlias 找不到匹配项，会把包路径原样保留，
		// 生成代码时 "hermes/internal/types.Agent" 会被解析为除法运算符序列
		var retPkgPath string
		if retPkg := e.typePkg(typeObj); retPkg != nil && retPkg.Path() != e.mainPkgPath {
			retPkgPath = retPkg.Path()
			e.aliasManager.EnsureAlias(retPkgPath)
		}
		targetType := e.replacePkgPathWithAlias(e.getTypeFullName(typeObj))
		item.IsIdentityClosure = true
		item.IdentityTargetType = targetType
		item.IdentityTargetPkg = retPkgPath
		item.IdentityOp = opType
		item.ShouldInline = false // identity collapse takes priority over IIFE
	}

	if retType != "" {
		item.RetType = retType
	}
	item.InstanceName = instanceName // 已在 4a 步骤提取

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

func (e *Extractor) validateClosureSignature(funcLit *ast.FuncLit, curPkg *packages.Package, isInvoke bool) (*types.Signature, error) {
	pos := curPkg.Fset.Position(funcLit.Pos())
	typ := curPkg.TypesInfo.TypeOf(funcLit)
	sig, ok := typ.(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("at %s: func literal is not a function type", pos)
	}
	if isInvoke {
		if err := validateInvokeSignature(sig, "anonymous function"); err != nil {
			return nil, fmt.Errorf("at %s: %w", pos, err)
		}
	} else {
		if err := validateProvideSignature(sig, "anonymous provide function"); err != nil {
			return nil, fmt.Errorf("at %s: %w", pos, err)
		}
	}
	return sig, nil
}

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

func (e *Extractor) registerClosureProvider(item extractedItem, idx int) error {
	key := item.RetType
	if item.InstanceName != "" {
		key = item.RetType + ":" + item.InstanceName
	}
	if oldIdx, exists := e.globalProviderMap[key]; exists {
		if oldIdx != idx {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("at %s: duplicate binding for %s with name %q:\n\tprevious: %s\n\tcurrent: %s",
				item.Position, item.RetType, item.InstanceName, oldDesc, currentDesc)
		}
	} else {
		e.globalProviderMap[key] = idx
	}
	return nil
}

func analyzeClosureInlinability(funcLit *ast.FuncLit, freeVars []*ast.Ident, isConst []bool) bool {
	// Condition 1: No non-const free variables
	for i := range freeVars {
		if i < len(isConst) && !isConst[i] {
			return false
		}
	}

	// Condition 2: No named return values
	if funcLit.Type.Results != nil {
		for _, field := range funcLit.Type.Results.List {
			if len(field.Names) > 0 {
				return false
			}
		}
	}

	// Condition 3: Single statement body
	// This is a heuristic to avoid overly complex IIFEs.
	// Multi-statement closures are kept as named functions for readability.
	if len(funcLit.Body.List) != 1 {
		return false
	}

	return true
}

func analyzeIdentityClosure(funcLit *ast.FuncLit, freeVars []*ast.Ident) (ast.Expr, model.OpKind) {
	// 1. 参数检查：必须恰好一个参数
	if funcLit.Type.Params == nil || len(funcLit.Type.Params.List) != 1 {
		return nil, ""
	}
	// 2. 返回值检查：必须恰好一个返回值
	if funcLit.Type.Results == nil || len(funcLit.Type.Results.List) != 1 {
		return nil, ""
	}
	// 3. 函数体必须只有一条 return 语句
	if len(funcLit.Body.List) != 1 {
		return nil, ""
	}
	retStmt, ok := funcLit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(retStmt.Results) != 1 {
		return nil, ""
	}

	// 4. 获取参数名
	param := funcLit.Type.Params.List[0]
	if len(param.Names) != 1 {
		return nil, ""
	}
	paramName := param.Names[0].Name

	// 5. 获取返回值类型表达式（用于返回）
	retTypeField := funcLit.Type.Results.List[0]
	if retTypeField == nil {
		return nil, ""
	}

	// 6. 分析返回表达式，确定操作类型
	expr := retStmt.Results[0]
	var op model.OpKind
	// 目标类型表达式：默认取返回类型；类型断言 p.(T) 的真实目标类型是断言类型 T，
	// 可能不同于返回类型（如 func(p any) Service { return p.(ServiceImpl) }），必须取 e.Type。
	targetTypeExpr := retTypeField.Type

	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == paramName {
			op = model.OpDirect
		}
	case *ast.UnaryExpr:
		switch e.Op {
		case token.AND:
			if ident, ok := e.X.(*ast.Ident); ok && ident.Name == paramName {
				op = model.OpAddr
			}
		case token.MUL:
			if ident, ok := e.X.(*ast.Ident); ok && ident.Name == paramName {
				op = model.OpDeref
			}
		}
	case *ast.StarExpr:
		// Go 解析器在某些上下文中将表达式 *x 表示为 StarExpr（与指针类型表示相同）
		// 这里兜底处理，确保解引用闭包检测正常工作
		if ident, ok := e.X.(*ast.Ident); ok && ident.Name == paramName {
			op = model.OpDeref
		}
	case *ast.CallExpr:
		if len(e.Args) == 1 {
			if ident, ok := e.Args[0].(*ast.Ident); ok && ident.Name == paramName {
				op = model.OpConvert
			}
		}
	case *ast.TypeAssertExpr:
		// 类型断言：x.(T) — 必须是单返回值的断言（e.Type 非 nil，排除 type switch 形），
		// 操作数为参数名。塌缩为内联断言 x.(T)，单次求值、断言失败同样 panic，与原闭包等价。
		if e.Type != nil {
			if ident, ok := e.X.(*ast.Ident); ok && ident.Name == paramName {
				op = model.OpAssert
				targetTypeExpr = e.Type // 断言类型 T，可能与返回类型不同
			}
		}
	}
	if op == "" {
		return nil, ""
	}

	// 7. 检查自由变量：不允许任何外部捕获
	if len(freeVars) > 0 {
		return nil, ""
	}

	// 8. 匹配成功，返回类型表达式和操作类型
	return targetTypeExpr, op
}

func (e *Extractor) checkMethodVisibilityInClosure(body *ast.BlockStmt, pkg *packages.Package) error {
	var err error
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.ObjectOf(sel.Sel)
		if obj == nil {
			return true
		}
		if visErr := e.checkGenerationVisibility(obj, pkg); visErr != nil {
			err = visErr
			return false
		}
		return true
	})
	return err
}

func (e *Extractor) checkFunctionVisibilityInClosure(body *ast.BlockStmt, pkg *packages.Package) error {
	var err error
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// 仅处理裸标识符调用（fn(args) / T(x)）；选择器调用（x.Method / pkg.Fn）
		// 由 checkMethodVisibilityInClosure 负责校验 sel.Sel 的可见性。
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.ObjectOf(ident)
		if obj == nil {
			return true
		}
		// checkGenerationVisibility 内部已放行：同包符号、导出符号、内建符号。
		if visErr := e.checkGenerationVisibility(obj, pkg); visErr != nil {
			err = visErr
			return false
		}
		return true
	})
	return err
}

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
	case reflect.Ptr:
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
		for _, pkgPath := range e.collectUsedPkgsFromType(t) {
			usedPkgs[pkgPath] = true
			e.aliasManager.EnsureAlias(pkgPath)
		}
	}

	paramList, freeVarMap, constMap := e.buildParamListAndFreeVarMap(it, usedPkgs)

	paramStr := strings.Join(paramList, ", ")

	rewrittenBody := e.replaceFreeVarsInBody(it.ClosureLit.Body, freeVarMap, constMap)

	typeNameMap := e.collectTypeNameAndUsedPkgs(rewrittenBody, it.Pkg, usedPkgs)

	// 在 AST 克隆上精确改写跨包标识符（不改动共享的源 AST）
	rewrittenBody = e.applyTypeAliasReplacements(rewrittenBody, typeNameMap)

	var bodyBuf bytes.Buffer
	if err := printer.Fprint(&bodyBuf, it.Pkg.Fset, rewrittenBody); err != nil {
		return "", nil, fmt.Errorf("printer print closure body failed: %w", err)
	}
	bodyStr := bodyBuf.String()
	bodyStr = e.replacePkgPathWithAlias(bodyStr)
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
	// 排序以保证生成结果（导入顺序、IR 序列化）可复现，避免 map 遍历顺序导致
	// 的不可确定性（例如同一闭包在不同时候产生不同的 UsedPkgs 顺序）。
	sort.Strings(usedList)
	comment := e.ConditionalDebugf(func() bool { return it.Pkg.PkgPath != e.mainPkgPath }, "// original package: %s\n", it.Pkg.PkgPath)
	def = comment + def
	return def, usedList, nil
}

// ensureAlias 确保指定包路径在 pkgAliasMap 中存在别名，如果不存在则生成并缓存。
// 若包在 pkgMap 中，则调用 collectPkgAlias（会基于策略和冲突处理生成）；
// 否则使用路径最后一段作为别名并缓存。
// 返回别名（若包路径为主包或空，返回空字符串）。

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

func (e *Extractor) buildNodes(order []int, items []extractedItem, varNames []string) ([]model.Node, error) {
	var final []model.Node
	for _, i := range order {
		it := items[i]
		argNames, err := e.resolveArgNames(it, varNames)
		if err != nil {
			return nil, err
		}
		switch {
		case it.IsInvoke:
			node, err := e.buildInvokeNode(it, argNames)
			if err != nil {
				return nil, err
			}
			final = append(final, node)
		case it.IsSupply:
			node, err := e.buildSupplyNode(it, varNames[i])
			if err != nil {
				return nil, err
			}
			final = append(final, node)
		default:
			node, err := e.buildProviderNode(it, argNames, varNames[i])
			if err != nil {
				return nil, err
			}
			final = append(final, node)
		}
	}
	return final, nil
}

// ---------- buildProviderNode 使用 baseNode ----------
func (e *Extractor) buildProviderNode(it extractedItem, argNames []string, name string) (model.Node, error) {
	node := e.baseNode(it, name, argNames)
	node.RetType = it.RetType
	node.HasError = it.HasError
	node.IsClosure = it.IsClosure
	node.ShouldInline = it.ShouldInline
	node.IsIdentityClosure = it.IsIdentityClosure
	node.IdentityTargetType = it.IdentityTargetType
	node.IdentityOp = it.IdentityOp
	node.Func = it.FuncName
	node.FuncPkg = it.PkgAlias
	if it.IsClosure {
		node.PkgPath = e.mainPkgPath
		closureDef, usedPkgs, err := e.generateClosureDef(&it)
		if err != nil {
			return model.Node{}, fmt.Errorf("generate closure definition: %w", err)
		}
		node.ClosureDef = closureDef
		// 身份闭包的目标类型所在包不会出现在 closureDef 的 usedPkgs 中（因为跳过了 generateClosureDef
		// 对返回类型的解析实际上会出现，但 OpAddr/OpDeref 情况下 retType=*/&T 与 closureDef 返回
		// 类型相同，为保险起见仍主动合并 IdentityTargetPkg）
		if it.IdentityTargetPkg != "" {
			found := slices.Contains(usedPkgs, it.IdentityTargetPkg)
			if !found {
				usedPkgs = append(usedPkgs, it.IdentityTargetPkg)
			}
		}
		node.UsedPkgs = usedPkgs
	}
	return node, nil
}

func (e *Extractor) buildSupplyNode(it extractedItem, name string) (model.Node, error) {
	var buf strings.Builder
	if err := printer.Fprint(&buf, it.Pkg.Fset, it.Expr); err != nil {
		return model.Node{}, fmt.Errorf("print supply expression: %w", err)
	}
	value := buf.String()

	// A free variable (function parameter or local of the inlined module) is
	// captured by the target function's own scope and referenced verbatim, so it
	// must NEITHER be package-qualified NOR pull an import for its defining
	// package. Package-level symbols and all other expression shapes keep the
	// historical behavior (qualified when inlined, and their defining package
	// remains an import).
	isFreeVar := isSupplyValueFreeVar(it.Expr, it.Pkg.TypesInfo)

	pkgPath := it.Pkg.PkgPath
	if isFreeVar {
		// The inlined supply code (e.g. `_ = cfg`) references only the target
		// scope, so do not import the source package.
		pkgPath = e.mainPkgPath
	}

	return model.Node{
		Name:             name,
		IsSupply:         true,
		Value:            value,
		ValueIsPkgSymbol: !isFreeVar && supplyValueNeedsQualification(it.Expr, it.Pkg.TypesInfo),
		FuncPkg:          it.PkgAlias,
		PkgPath:          pkgPath,
		RetType:          it.RetType,
		UsedPkgs:         it.UsedPkgs,
		Comment:          it.SourceComment,
	}, nil
}

// supplyValueNeedsQualification reports whether the Supply value expression,
// once inlined into the generation target package, must be package-qualified
// (prefixed with `<FuncPkg>.`).
//
// The ONLY case that must NOT be qualified is a bare identifier that names a
// free variable — a function parameter or a local variable of the inlined
// module. Such identifiers are captured by the target function's own scope and
// referenced verbatim, so prefixing them with FuncPkg yields an undefined
// reference (e.g. `undefined: supply_param_helper.cfg`).
//
// Every other expression shape preserves the historical qualification behavior:
//   - bare identifier naming a package-level symbol (var/func/const/type): qualify
//     (e.g. `Index` -> `db.Index`, `Config` -> `role.Config`);
//   - type-conversion / function calls whose Fun is a package-level symbol:
//     qualify the Fun (e.g. `Config("production")` -> `role.Config("production")`);
//   - selector expressions (`pkg.Sym`): already carry their package, the
//     existing HasPrefix guard skips them;
//   - composite literals (`&pkg.T{...}`): already carry their package;
//   - literals: skipped by isLiteral.
//
// Returning true for every non-bare-identifier shape keeps the long-standing
// behavior intact and only narrows the false-positive on free variables.
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
