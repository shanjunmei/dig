package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/packages"
	"slices"
)

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

func (e *Extractor) handleSupply(expr ast.Expr, curPkg *packages.Package) error {
	pos := curPkg.Fset.Position(expr.Pos())
	if e.isDigOptionCall(expr, curPkg.TypesInfo) {
		return fmt.Errorf("at %s: dig.Supply cannot accept another Option as argument; only dig.Module can nest Options", pos)
	}
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := e.checkGenerationVisibility(obj, curPkg); err != nil {
			return err
		}
	}
	alias := e.aliasManager.CollectPkgAlias(curPkg)
	typ := curPkg.TypesInfo.TypeOf(expr)
	if typ == nil {
		return fmt.Errorf("at %s: resolve supply type failed", pos)
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
	// 检查键冲突：命名实例和默认实例使用不同的错误信息
	if instanceName != "" {
		if oldIdx, exists := e.globalProviderMap[keyNamed]; exists {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("at %s: duplicate binding for %s with name %q:\n\tprevious: %s\n\tcurrent: %s",
				pos, retType, instanceName, oldDesc, currentDesc)
		}
	} else {
		if oldIdx, exists := e.globalProviderMap[keyDefault]; exists {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("at %s: duplicate binding for %s (default):\n\tprevious: %s\n\tcurrent: %s",
				pos, retType, oldDesc, currentDesc)
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

func (e *Extractor) handleInvoke(expr ast.Expr, curPkg *packages.Package) error {
	if e.isDigOptionCall(expr, curPkg.TypesInfo) {
		pos := curPkg.Fset.Position(expr.Pos())
		return fmt.Errorf("at %s: dig.Invoke cannot accept another Option as argument; only dig.Module can nest Options", pos)
	}
	if funcLit, ok := expr.(*ast.FuncLit); ok {
		return e.handleFuncLit(funcLit, curPkg, true)
	}
	name, sig, realPkg, err := getFuncMeta(expr, curPkg, e.pkgMap)
	if err != nil {
		return err
	}
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := e.checkGenerationVisibility(obj, curPkg); err != nil {
			return err
		}
	}
	if err := validateInvokeSignature(sig, name); err != nil {
		pos := curPkg.Fset.Position(expr.Pos())
		return fmt.Errorf("at %s: %w", pos, err)
	}
	genericStr, err := e.extractGenericArgStr(expr, curPkg)
	if err != nil {
		return err
	}
	alias := e.aliasManager.CollectPkgAlias(realPkg)
	hasErr := sigHasError(sig)
	item := e.newExtractedItem(name, realPkg, alias, hasErr)
	item.IsInvoke = true
	item.Params = e.buildExtractedParams(sig) // 注意这里现在保留了参数名
	item.GenericArgsStr = genericStr
	e.items = append(e.items, item)
	return nil
}

func (e *Extractor) handleProvide(expr ast.Expr, curPkg *packages.Package) error {
	if e.isDigOptionCall(expr, curPkg.TypesInfo) {
		pos := curPkg.Fset.Position(expr.Pos())
		return fmt.Errorf("at %s: dig.Provide cannot accept another Option as argument; only dig.Module can nest Options", pos)
	}
	if funcLit, ok := expr.(*ast.FuncLit); ok {
		return e.handleFuncLit(funcLit, curPkg, false)
	}
	name, sig, realPkg, err := getFuncMeta(expr, curPkg, e.pkgMap)
	if err != nil {
		return err
	}
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj != nil {
		if err := e.checkGenerationVisibility(obj, curPkg); err != nil {
			return err
		}
	}
	genericStr, err := e.extractGenericArgStr(expr, curPkg)
	if err != nil {
		return err
	}
	alias := e.aliasManager.CollectPkgAlias(realPkg)

	pos := curPkg.Fset.Position(expr.Pos())
	res := sig.Results()
	switch res.Len() {
	case 0:
		return fmt.Errorf("at %s: func %s has no return", pos, name)
	case 1:
		// ok
	case 2:
		if !isErrorType(res.At(1).Type()) {
			return fmt.Errorf("at %s: func %s: second return value must be error, got %s", pos, name, res.At(1).Type().String())
		}
	default:
		return fmt.Errorf("at %s: func %s: too many return values (%d), only (T) or (T, error) are allowed "+
			"(if you need to provide multiple types, define a plain struct that bundles them and return that struct)", pos, name, res.Len())
	}

	retType := e.getTypeFullName(res.At(0).Type())
	hasErr := sigHasError(sig)
	instanceName := e.extractNamedReturn(sig)

	item := e.newExtractedItem(name, realPkg, alias, hasErr)
	item.RetType = retType
	item.Params = e.buildExtractedParams(sig) // 保留参数名
	item.GenericArgsStr = genericStr
	item.InstanceName = instanceName

	// Providers cannot take a context.Context parameter (resolved before ctx exists).
	if err := e.checkProviderContextParams(item, pos); err != nil {
		return err
	}

	relPath := e.relPath(pos.Filename)
	item.Position = fmt.Sprintf("%s:%d", relPath, pos.Line)

	keyNamed := retType
	if instanceName != "" {
		keyNamed = retType + ":" + instanceName
	}
	// 检查命名键冲突
	if oldIdx, exists := e.globalProviderMap[keyNamed]; exists {
		oldDesc := e.describeItem(oldIdx)
		currentDesc := e.describeItemByIt(item)
		return fmt.Errorf("at %s: duplicate binding for %s with name %q:\n\tprevious: %s\n\tcurrent: %s",
			pos, retType, instanceName, oldDesc, currentDesc)
	}
	if instanceName == "" {
		if oldIdx, exists := e.globalProviderMap[retType]; exists {
			oldDesc := e.describeItem(oldIdx)
			currentDesc := e.describeItemByIt(item)
			return fmt.Errorf("at %s: duplicate binding for %s (default):\n\tprevious: %s\n\tcurrent: %s",
				pos, retType, oldDesc, currentDesc)
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

func (e *Extractor) extractOptionsFromFuncCall(call *ast.CallExpr, curPkg *packages.Package) error {
	pos := curPkg.Fset.Position(call.Pos())
	obj := resolveFunctionObject(call, curPkg)
	if obj == nil {
		return fmt.Errorf("at %s: cannot resolve function call; ensure it is a named function or method, not a literal or variable\n  💡 Fix: define a named function with dig.Module(...) and call it", pos)
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return fmt.Errorf("at %s: resolved object is not a function", pos)
	}
	fnPkg := fn.Pkg()
	if fnPkg == nil {
		return fmt.Errorf("at %s: function has no package", pos)
	}
	if err := e.checkGenerationVisibility(obj, curPkg); err != nil {
		return err
	}
	subPkg, ok := e.pkgMap[fnPkg.Path()]
	if !ok {
		return fmt.Errorf("at %s: package %s not loaded", pos, fnPkg.Path())
	}
	funcDecl := findFuncDecl(subPkg, fn.Name())
	if funcDecl == nil || funcDecl.Body == nil {
		return fmt.Errorf("at %s: function %s has no body", pos, fn.Name())
	}

	modCalls, err := e.findAllModuleCalls(funcDecl.Body, subPkg.TypesInfo, fn.Name(), subPkg.Fset)
	if err != nil {
		return err
	}

	for _, modCall := range modCalls {
		for _, arg := range modCall.Args {
			if err := e.extractOptions(arg, subPkg, subPkg); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Extractor) ExtractOptions(expr ast.Expr, curPkg, realPkg *packages.Package) error {
	return e.extractOptions(expr, curPkg, realPkg)
}

func (e *Extractor) resolveProvider(arg ExtractedArg, it extractedItem) (int, error) {
	if idx, ok := e.resolveProviderIndex(arg); ok {
		return idx, nil
	}
	return 0, e.buildProviderNotFoundError(arg.TypeString, e.getRequiredInstanceName(arg), it)
}

func (e *Extractor) resolveArgNames(it extractedItem, varNames []string) ([]string, error) {
	argNames := make([]string, len(it.Params))
	for j, arg := range it.Params {
		if arg.IsContext {
			argNames[j] = ""
			continue
		}
		if arg.IsConst {
			argNames[j] = arg.ConstValue
			continue
		}
		idx, ok := e.resolveProviderIndex(arg)
		if !ok {
			return nil, e.buildProviderNotFoundError(arg.TypeString, e.getRequiredInstanceName(arg), it)
		}
		argNames[j] = varNames[idx]
	}
	return argNames, nil
}

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
	nodes, err := e.buildNodes(order, items, varNames)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

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
		Position:    it.Position,
	}
}

func (e *Extractor) buildInvokeNode(it extractedItem, argNames []string) (model.Node, error) {
	node := e.baseNode(it, "", argNames)
	node.IsInvoke = true
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
