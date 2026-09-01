package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
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
			// Skip the capture check ONLY for cross-package exported symbols
			// (e.g. pkg.ExportedVar). A same-package exported variable already
			// has o.Parent() == pkgScope, so it falls through to the
			// o.Parent() != pkgScope test below and is naturally treated as a
			// legitimate package-level reference — it never triggers a false
			// "cannot capture local variable" error. Restricting this guard to
			// o.Pkg().Path() != e.mainPkgPath makes the intent explicit: this
			// branch exists solely to whitelist cross-package exported symbols
			// whose Parent() lives in a *different* package scope than the
			// current one.
			if o.Exported() && o.Pkg() != nil && o.Pkg().Path() != e.mainPkgPath {
				return true
			}
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
			// Same reasoning as the *types.Var branch above: only cross-package
			// exported constants (e.g. pkg.ExportedConst) are whitelisted here.
			// A same-package exported constant has o.Parent() == pkgScope and is
			// handled correctly by the o.Parent() != pkgScope test below, so it
			// never produces a false "cannot capture local constant" error. The
			// o.Pkg().Path() != e.mainPkgPath guard makes it explicit that this
			// branch targets cross-package exported symbols only.
			if o.Exported() && o.Pkg() != nil && o.Pkg().Path() != e.mainPkgPath {
				return true
			}
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
	if retTypeExpr, opType := analyzeIdentityClosure(funcLit, freeVars, curPkg.TypesInfo); retTypeExpr != nil {
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

func analyzeIdentityClosure(funcLit *ast.FuncLit, freeVars []*ast.Ident, typeInfo *types.Info) (ast.Expr, model.OpKind) {
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
				// Only treat `T(param)` as a conversion when Fun resolves to a TYPE.
				// A function call like `return NewFoo(param)` has Fun resolving to a
				// *types.Func and is NOT an identity conversion — collapsing it would
				// emit `Foo(dvN)` and break compilation. (TypeInfo may be nil in some
				// unit-test call paths; fall back to the previous lenient behavior.)
				if typeInfo != nil {
					var funIdent *ast.Ident
					switch f := e.Fun.(type) {
					case *ast.Ident:
						funIdent = f
					case *ast.SelectorExpr:
						funIdent = f.Sel
					}
					if funIdent != nil {
						if obj := typeInfo.ObjectOf(funIdent); obj != nil {
							if _, ok := obj.(*types.TypeName); ok {
								op = model.OpConvert
							}
						}
					}
				} else {
					op = model.OpConvert
				}
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
