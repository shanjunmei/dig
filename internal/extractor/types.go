package extractor

import (
	"bytes"
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/functional"
	"go/ast"
	"go/constant"
	"go/printer"
	"go/types"
	"golang.org/x/tools/go/packages"
	"sort"
	"strconv"
	"strings"
)

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

func (e *Extractor) extractConstLiteral(c *types.Const) string {
	val := c.Val()
	switch val.Kind() {
	case constant.String:
		return strconv.Quote(val.String())
	default:
		return val.String()
	}
}

func (e *Extractor) addPkgToUsed(typ types.Type, usedPkgs map[string]bool) {
	for _, pkgPath := range e.collectUsedPkgsFromType(typ) {
		usedPkgs[pkgPath] = true
	}
}

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

func isErrorType(typ types.Type) bool {
	return types.Identical(typ, errorType)
}

func isContextType(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r := rune(name[0])
	return r >= 'A' && r <= 'Z'
}

func (e *Extractor) typeQualifier(p *types.Package) string {
	return p.Path()
}

func (e *Extractor) isDigOptionCall(expr ast.Expr, info *types.Info) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	obj := info.ObjectOf(sel.Sel)
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	if obj.Pkg().Path() != diPkgPath {
		return false
	}
	switch obj.Name() {
	case "Provide", "Invoke", "Supply", "Module":
		return true
	}
	return false
}

func (e *Extractor) getTypeFullName(typ types.Type) string {
	if s, ok := e.typeStrCache[typ]; ok {
		return s
	}
	s := types.TypeString(typ, e.typeQualifier)
	e.typeStrCache[typ] = s
	return s
}

func (e *Extractor) ImportAliasMap() map[string]string {
	return e.aliasManager.GetImportAliasMap()
}

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
			// 实例化级：遍历类型实参（Cache[*common.Config] → 遍历 *common.Config）
			if args := t.TypeArgs(); args != nil {
				for t := range args.Types() {
					walk(t)
				}
			}
			// 声明级：遍历类型参数约束（泛型定义本身的约束里可能引用跨包）
			if params := t.TypeParams(); params != nil {
				for tparam := range params.TypeParams() {
					walk(tparam)
				}
			}
			// 注意：不调用 walk(t.Underlying())，因为自引用类型（如 type Node struct{ Next *Node }）
			// 会导致无限递归。struct 字段/方法签名中的跨包引用由 collectTypeNameAndUsedPkgs
			// 的 AST 遍历覆盖，不需要在此重复。
		case *types.Pointer, *types.Slice, *types.Array, *types.Chan:
			walk(t.(interface{ Elem() types.Type }).Elem())
		case *types.Map:
			walk(t.Key())
			walk(t.Elem())
		case *types.Signature:
			// 函数/方法签名:遍历接收者、参数和返回值类型,收集其中的跨包引用
			// 例如 func(*common.Config) error → 遍历 *common.Config 和 error
			// Recv 通常与外层 *types.Named 相同,已被 seen 去重,遍历是安全的
			if recv := t.Recv(); recv != nil {
				walk(recv.Type())
			}
			for i := 0; i < t.Params().Len(); i++ {
				walk(t.Params().At(i).Type())
			}
			for i := 0; i < t.Results().Len(); i++ {
				walk(t.Results().At(i).Type())
			}
			// 泛型方法/函数的类型参数约束里可能引用跨包类型
			if tparams := t.TypeParams(); tparams != nil {
				for i := 0; i < tparams.Len(); i++ {
					walk(tparams.At(i).Constraint())
				}
			}
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

func (e *Extractor) populateUsedPkgs() {
	for i := range e.items {
		it := &e.items[i]
		if it.IsClosure {
			continue
		}
		if len(it.UsedPkgs) > 0 {
			// 已有 UsedPkgs 的 item 仍需提前注册别名（如 Supply 在提取阶段已填充 UsedPkgs）
			for _, pkgPath := range it.UsedPkgs {
				e.aliasManager.EnsureAlias(pkgPath)
			}
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

		// 提前注册别名，确保后续 ShadowGuard 可见
		for pkgPath := range usedMap {
			e.aliasManager.EnsureAlias(pkgPath)
		}

		// 转为切片（排序以保证可复现，避免 map 遍历顺序不确定）
		used := make([]string, 0, len(usedMap))
		for p := range usedMap {
			used = append(used, p)
		}
		sort.Strings(used)
		it.UsedPkgs = used
	}
}

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

func (e *Extractor) getRequiredInstanceName(arg ExtractedArg) string {
	if arg.Name != "" && arg.Name != "_" {
		return arg.Name
	}
	return ""
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
	pos := curPkg.Fset.Position(expr.Pos())
	obj := resolveFunctionObject(&ast.CallExpr{Fun: expr}, curPkg)
	if obj == nil {
		var buf strings.Builder
		_ = printer.Fprint(&buf, curPkg.Fset, expr)
		return "", nil, nil, fmt.Errorf("at %s: resolve object failed for expression: %s", pos, buf.String())
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return "", nil, nil, fmt.Errorf("at %s: %s is not a function", pos, obj.Name())
	}
	fnPkg := fn.Pkg()
	if fnPkg == nil {
		return "", nil, nil, fmt.Errorf("at %s: function %s has no package", pos, fn.Name())
	}
	realPkg, ok = pkgMap[fnPkg.Path()]
	if !ok {
		return "", nil, nil, fmt.Errorf("at %s: package %s not found in pkgMap", pos, fnPkg.Path())
	}
	instFuncType := curPkg.TypesInfo.TypeOf(expr)
	instSig, ok := instFuncType.(*types.Signature)
	if !ok {
		return "", nil, nil, fmt.Errorf("at %s: failed to get instantiated signature for %s", pos, fn.Name())
	}

	return fn.Name(), instSig, realPkg, nil
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
	mainPrefix := e.aliasManager.GetMainPkgPath() + "."
	typeStr = strings.ReplaceAll(typeStr, mainPrefix, "")

	pairs := functional.MapEntries(e.aliasManager.GetPkgAliasMap(), func(path, alias string) pair {
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
