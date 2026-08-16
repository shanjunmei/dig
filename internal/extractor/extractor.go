package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/alias"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	aliasManager      *AliasManager
	typeStrCache      map[types.Type]string
	invokeIndex       int
	provideIndex      int
	moduleRoot        string
	cfg               *config.Config
	logger            *logger.Logger
}

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

	ShouldInline bool // Phase 3: inlining candidate

	// ---------- 新增字段 ----------
	InstanceName string // 实例名称（命名返回值或 Supply 表达式名称）

	IsIdentityClosure  bool // Phase 4: identity closure (func(param) T { return param })
	IdentityOp         model.OpKind
	IdentityTargetType string // Phase 4: target type for identity conversion
	IdentityTargetPkg  string // Phase 4: 返回类型所在包路径，用于合并到 UsedPkgs
}

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

func NewExtractor(cfg *config.Config, pkgMap map[string]*packages.Package, mainPkgPath string, strategy alias.AliasStrategy, startDir string, logger *logger.Logger) *Extractor {
	rootDir := findModuleRoot(startDir)
	e := &Extractor{
		cfg:               cfg,
		pkgMap:            pkgMap,
		mainPkgPath:       mainPkgPath,
		items:             []extractedItem{},
		globalProviderMap: make(map[string]int),
		logger:            logger,
		typeStrCache:      make(map[types.Type]string),

		moduleRoot: rootDir,
	}
	e.aliasManager = NewAliasManager(mainPkgPath, strategy, pkgMap, logger)
	e.aliasManager.LoadImportAliases()
	return e
}

func (e *Extractor) ConditionalDebugf(pred func() bool, tpl string, args ...any) string {
	if !e.cfg.Debug || !pred() {
		return ""
	}
	return fmt.Sprintf(tpl, args...)
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

var errorType = types.Universe.Lookup("error").Type()

func (e *Extractor) PackageNameMap() map[string]string {
	return e.aliasManager.GetPkgNameMap()
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

func validateProvideSignature(sig *types.Signature, funcName string) error {
	res := sig.Results()
	switch res.Len() {
	case 0:
		return fmt.Errorf("func %s has no return", funcName)
	case 1:
		return nil
	case 2:
		if !isErrorType(res.At(1).Type()) {
			return fmt.Errorf("func %s: second return value must be error, got %s", funcName, res.At(1).Type().String())
		}
		return nil
	default:
		return fmt.Errorf("func %s: too many return values (%d), only (T) or (T, error) are allowed "+
			"(if you need to provide multiple types, define a plain struct that bundles them and return that struct)", funcName, res.Len())
	}
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

func (e *Extractor) findAllModuleCalls(body *ast.BlockStmt, info *types.Info, funcName string, fset *token.FileSet) ([]*ast.CallExpr, error) {
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

	pos := fset.Position(body.Pos())
	if len(moduleCalls) == 0 {
		return nil, fmt.Errorf("at %s: function %s does not contain dig.Module\n  💡 Fix: add a dig.Module(...) call that wraps all dig.Provide/dig.Invoke/dig.Supply calls", pos, funcName)
	}
	for i, inControl := range moduleInControl {
		if inControl {
			modPos := fset.Position(moduleCalls[i].Pos())
			return nil, fmt.Errorf("at %s: function %s contains dig.Module inside control flow (if/switch/for/select), which is not supported\n  💡 Fix: pass it as a parameter to the function (preferred) or move it to package level", modPos, funcName)
		}
	}
	return moduleCalls, nil
}

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
		var remaining []int
		for i := range n {
			if indeg[i] > 0 {
				remaining = append(remaining, i)
			}
		}
		return nil, fmt.Errorf("circular dependency detected involving %d node(s)", len(remaining))
	}
	return order, nil
}

type pair struct {
	path  string
	alias string
}
