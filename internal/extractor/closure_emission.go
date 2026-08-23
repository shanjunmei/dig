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
	"regexp"
	"slices"
	"sort"
	"strings"
)

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
