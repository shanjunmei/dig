package extractor

import (
	"bytes"
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/functional"
	"go/ast"
	"go/printer"
	"go/types"
	"golang.org/x/tools/go/packages"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
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

// extractConstLiteral 返回常量的 Go 表达式形式，供闭包内联时替换常量引用。
// constant.Value.String() 对字符串常量已返回带引号形式（如 "font:cjk"），
// 对其它类型返回合法字面量，因此直接返回即可；再套 strconv.Quote 会产生
// 二次转义（"\"font:cjk\""），导致内联后的字符串多出一层引号。
func (e *Extractor) extractConstLiteral(c *types.Const) string {
	return c.Val().String()
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
				for i := 0; i < args.Len(); i++ {
					walk(args.At(i))
				}
			}
			// 声明级：遍历类型参数约束（泛型定义本身的约束里可能引用跨包）
			if params := t.TypeParams(); params != nil {
				for i := 0; i < params.Len(); i++ {
					walk(params.At(i))
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
			for i := 0; i < t.NumFields(); i++ {
				walk(t.Field(i).Type())
			}
		case *types.Interface:
			for i := 0; i < t.NumMethods(); i++ {
				walk(t.Method(i).Type())
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
		if err := printer.Fprint(&buf, curPkg.Fset, expr); err != nil {
			buf.WriteString("<unprintable expression>")
		}
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
		// A provider function whose defining package is missing from the loaded
		// graph. The most common cause is a dependency that does not compile:
		// go/packages does not fully load a package with compile errors, so its
		// exported symbols (and this provider) are invisible here. Make the fix
		// actionable instead of dumping a bare "not found".
		return "", nil, nil, fmt.Errorf(
			"at %s: package %s is not available in the loaded package graph\n  💡 Fix: ensure %s is required in go.mod and that `go build ./...` compiles it cleanly (packages with compile errors are not fully loaded, so their providers cannot be extracted)",
			pos, fnPkg.Path(), fnPkg.Path())
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

	// 注意：替换必须「标识符边界敏感」，且幂等（不重扫新插入的文本）。
	// 若沿用朴素 ReplaceAll("<path>.", "<alias>.")，当别名本身包含包路径时
	// （例如源码写 `import gotime "time"` 且闭包体里已是 gotime.Duration），
	// "gotime." 内部的 "time." 会被再替换一次，得到错误的 "gogotime."，
	// 进而让生成文件出现 undefined: gogotime。
	for _, p := range pairs {
		if p.path == p.alias {
			continue
		}
		typeStr = replaceQualifierOnce(typeStr, p.path, p.alias)
	}

	return prefix.String() + typeStr
}

// replaceQualifierOnce 把形如 "<path>." 的包限定符替换为 "<alias>."，但仅当该
// 出现位置的前一个字符**不是**标识符字符（字母/数字/下划线）时才替换。
//
// 这保证两件事：
//  1. 幂等——已写成别名的文本不会被再替换一次（如 gotime.Duration 不会被改成
//     gogotime.Duration，因为其中的 "time." 前置字符是 'o'）；
//  2. 精确——不会把更长标识符中的子串当包限定符（如 mytimeUtil 中的 time.）。
//
// 前置字符必须按「符文」而非「字节」判断：Go 标识符允许 Unicode 字母/数字
// （如 `时` `Ω`），而多字节 UTF-8 的每个字节都 >= 0x80，用字节比对会把它们
// 一律当成非标识符字符，从而错误改写 `用时time.Duration` 这类标识符
// （`用时time` 是同一个标识符）并污染字符串字面量与注释。
func replaceQualifierOnce(s, path, alias string) string {
	needle := path + "."
	if !strings.Contains(s, needle) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	from := 0
	for {
		idx := strings.Index(s[from:], needle)
		if idx < 0 {
			break
		}
		idx += from
		end := idx + len(needle)
		if idx > 0 {
			prev, _ := utf8.DecodeLastRuneInString(s[:idx])
			if isIdentRune(prev) {
				// 属于更长标识符的一部分，原样保留
				b.WriteString(s[from:end])
				from = end
				continue
			}
		}
		b.WriteString(s[from:idx])
		b.WriteString(alias)
		b.WriteString(".")
		from = end
	}
	b.WriteString(s[from:])
	return b.String()
}

// isIdentRune 判断 r 是否为可出现在 Go 标识符（非首字符位置）中的字符，依据
// Go 规范：letter = unicode_letter | "_" ；unicode_digit = 类别 Nd 的 Unicode 字符。
//
// 非法 UTF-8 时 DecodeLastRuneInString 返回 utf8.RuneError，此处判为非标识符字符。
func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
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
