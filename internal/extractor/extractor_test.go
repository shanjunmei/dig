package extractor

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/alias"
	"golang.org/x/tools/go/packages"
)

// validTopoOrder reports whether `order` is a valid topological ordering of a
// graph with `n` nodes and adjacency list `adj` (edge u->v means u must come
// before v).
func validTopoOrder(n int, adj [][]int, order []int) bool {
	if len(order) != n {
		return false
	}
	pos := make([]int, n)
	for i, u := range order {
		pos[u] = i
	}
	for u := 0; u < n; u++ {
		for _, v := range adj[u] {
			if pos[u] >= pos[v] {
				return false
			}
		}
	}
	return true
}

func TestTopologicalSort_Acyclic(t *testing.T) {
	// 0->1->2->3 (a line)
	adj := [][]int{{1}, {2}, {3}, {}}
	indeg := []int{0, 1, 1, 1}
	order, err := topologicalSort(4, adj, indeg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !validTopoOrder(4, adj, order) {
		t.Fatalf("invalid topo order: %v", order)
	}
}

func TestTopologicalSort_Diamond(t *testing.T) {
	// 0->1, 0->2, 1->3, 2->3
	adj := [][]int{{1, 2}, {3}, {3}, {}}
	indeg := []int{0, 1, 1, 2}
	order, err := topologicalSort(4, adj, indeg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !validTopoOrder(4, adj, order) {
		t.Fatalf("invalid topo order: %v", order)
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	// 0->1->2->0
	adj := [][]int{{1}, {2}, {0}}
	indeg := []int{1, 1, 1}
	order, err := topologicalSort(3, adj, indeg)
	if err == nil {
		t.Fatalf("expected cycle error, got order %v", order)
	}
	if order != nil {
		t.Fatalf("expected nil order on cycle, got %v", order)
	}
}

func TestFindCycle(t *testing.T) {
	e := &Extractor{}

	// acyclic -> nil
	if cyc := e.findCycle([][]int{{1}, {2}, {3}, {}}); cyc != nil {
		t.Fatalf("expected nil cycle for acyclic graph, got %v", cyc)
	}

	// cyclic 0->1->2->0 -> non-nil, len >= 2
	cyc := e.findCycle([][]int{{1}, {2}, {0}})
	if cyc == nil {
		t.Fatalf("expected a cycle for 0->1->2->0, got nil")
	}
	if len(cyc) < 2 {
		t.Fatalf("cycle too short: %v", cyc)
	}
}

// TestApplyAliasRewritesOnClone proves the core fix for #6: the alias rewrite is
// done on AST identifiers resolved by type info, so it can never corrupt string
// literals or comments, nor rewrite a same-named local variable. It also proves
// the shared source AST is left untouched (the rewrite happens on a clone).
func TestApplyAliasRewritesOnClone(t *testing.T) {
	src := `package p
func _() string {
	s := Bootstrap()
	_ = fmt.Sprintf("Bootstrap done")
	return s
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	body := fn.Body

	// Find the bare "Bootstrap" ident (the call), which is what needs aliasing.
	var bootstrap *ast.Ident
	qualified := map[*ast.Ident]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			qualified[sel.Sel] = true
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "Bootstrap" && !qualified[id] && bootstrap == nil {
			bootstrap = id
		}
		return true
	})
	if bootstrap == nil {
		t.Fatalf("bare Bootstrap ident not found")
	}

	posRepl := map[token.Pos]string{bootstrap.Pos(): "helper.Bootstrap"}

	e := &Extractor{}
	clone := e.applyTypeAliasReplacements(body, posRepl)

	// Original source AST must be untouched: the bare Bootstrap call still references
	// the same *ast.Ident (not a SelectorExpr), because the rewrite happened on a clone.
	var found bool
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id == bootstrap {
				found = true
			}
		}
		return true
	})
	if !found {
		t.Fatalf("source AST was mutated: bare Bootstrap call no longer references the original ident")
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, clone); err != nil {
		t.Fatalf("print: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "helper.Bootstrap()") {
		t.Fatalf("expected aliased call helper.Bootstrap(), got:\n%s", got)
	}
	if !strings.Contains(got, `"Bootstrap done"`) {
		t.Fatalf("string literal was dropped, got:\n%s", got)
	}
	if strings.Contains(got, "helper.Bootstrap done") {
		t.Fatalf("string literal was corrupted by the rewrite, got:\n%s", got)
	}
	if !strings.Contains(got, "return s") {
		t.Fatalf("local variable s was rewritten, got:\n%s", got)
	}
}

// TestCollectTypeNameAndUsedPkgs exercises the resolution side of #6 with a real,
// type-checked package: a closure defined in package "helper" references helper's
// own package-level func/type bare (Bootstrap, Config) and a qualified call
// (fmt.Sprintf). It asserts that only the bare references are recorded for alias
// rewriting, the qualified .Sel is skipped, and cross-package imports are tracked.
func TestCollectTypeNameAndUsedPkgs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "helper/helper.go", `package helper

import "fmt"

func Bootstrap() string { return "x" }

type Config struct{ Name string }

var Closure = func() string {
	s := Bootstrap()
	_ = fmt.Sprintf("Bootstrap done")
	_ = Config{}
	_ = s
	return s
}
`)
	writeTestFile(t, dir, "main/main.go", `package main

import "testmod/helper"

var _ = helper.Closure
`)

	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  dir,
	}
	loaded, err := packages.Load(cfg, "testmod/helper", "testmod/main")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	for _, p := range loaded {
		if len(p.Errors) > 0 {
			t.Fatalf("load errors for %s: %v", p.PkgPath, p.Errors)
		}
	}

	pkgMap := map[string]*packages.Package{}
	var add func(p *packages.Package)
	add = func(p *packages.Package) {
		if p == nil || p.PkgPath == "" {
			return
		}
		if _, ok := pkgMap[p.PkgPath]; ok {
			return
		}
		pkgMap[p.PkgPath] = p
		for _, imp := range p.Imports {
			add(imp)
		}
	}
	for _, p := range loaded {
		add(p)
	}

	helperPkg := pkgMap["testmod/helper"]
	if helperPkg == nil {
		t.Fatalf("helper package not loaded")
	}

	var body *ast.BlockStmt
	findClosureBody(helperPkg, &body)
	if body == nil {
		t.Fatalf("Closure body not found")
	}

	am := NewAliasManager("testmod/main", alias.SimpleAliasStrategy{}, pkgMap, &logger.Logger{})
	am.LoadImportAliases()
	e := &Extractor{mainPkgPath: "testmod/main", aliasManager: am}

	usedPkgs := map[string]bool{}
	posRepl := e.collectTypeNameAndUsedPkgs(body, helperPkg, usedPkgs)

	if len(posRepl) != 2 {
		t.Fatalf("expected 2 rewrite entries (Bootstrap, Config), got %d: %v", len(posRepl), posRepl)
	}
	values := map[string]bool{}
	for _, v := range posRepl {
		values[v] = true
	}
	if !values["helper.Bootstrap"] {
		t.Fatalf("missing helper.Bootstrap in %v", posRepl)
	}
	if !values["helper.Config"] {
		t.Fatalf("missing helper.Config in %v", posRepl)
	}
	if !usedPkgs["testmod/helper"] {
		t.Fatalf("expected testmod/helper in usedPkgs")
	}
	if !usedPkgs["fmt"] {
		t.Fatalf("expected fmt in usedPkgs")
	}

	clone := e.applyTypeAliasReplacements(body, posRepl)
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, helperPkg.Fset, clone); err != nil {
		t.Fatalf("print: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "helper.Bootstrap()") {
		t.Fatalf("expected helper.Bootstrap() in output:\n%s", out)
	}
	if !strings.Contains(out, "helper.Config{}") {
		t.Fatalf("expected helper.Config{} in output:\n%s", out)
	}
	if !strings.Contains(out, `"Bootstrap done"`) {
		t.Fatalf("string literal dropped:\n%s", out)
	}
	if strings.Contains(out, "helper.Bootstrap done") {
		t.Fatalf("string literal corrupted:\n%s", out)
	}
}

func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func findClosureBody(pkg *packages.Package, dst **ast.BlockStmt) {
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, n := range vs.Names {
					if n.Name != "Closure" {
						continue
					}
					if i >= len(vs.Values) {
						continue
					}
					if fl, ok := vs.Values[i].(*ast.FuncLit); ok {
						*dst = fl.Body
						return
					}
				}
			}
		}
	}
}

// TestIdentityClosureAlwaysAppliedWithoutInline locks the Option A invariant:
// identity-closure collapse (Phase 4) must run even when -inline (IIFE, Phase 3)
// is OFF. A regression that re-couples the two behind the -inline switch would
// silently stop collapsing identity closures for the default `digen ./...`
// invocation (no flags). Here we drive the real extractor path (handleFuncLit)
// with InlineClosures=false and assert the closure is still marked as identity.
func TestIdentityClosureAlwaysAppliedWithoutInline(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main/main.go", `package main

type T struct{}

var Closure = func(p T) T { return p }
`)

	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	loaded, err := packages.Load(cfg, "testmod/main")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(loaded) == 0 || loaded[0].PkgPath == "" {
		t.Fatalf("main package not loaded")
	}
	if len(loaded[0].Errors) > 0 {
		t.Fatalf("load errors for %s: %v", loaded[0].PkgPath, loaded[0].Errors)
	}
	pkg := loaded[0]

	pkgMap := map[string]*packages.Package{pkg.PkgPath: pkg}
	am := NewAliasManager(pkg.PkgPath, alias.SimpleAliasStrategy{}, pkgMap, &logger.Logger{})
	am.LoadImportAliases()

	e := &Extractor{
		cfg:               &config.Config{InlineClosures: false}, // IIFE OFF — only identity should apply
		pkgMap:            pkgMap,
		mainPkgPath:       pkg.PkgPath,
		aliasManager:      am,
		globalProviderMap: make(map[string]int),
		typeStrCache:      make(map[types.Type]string),
	}

	funcLit := findClosureFuncLit(pkg, "Closure")
	if funcLit == nil {
		t.Fatalf("Closure funcLit not found")
	}

	if err := e.handleFuncLit(funcLit, pkg, false); err != nil {
		t.Fatalf("handleFuncLit: %v", err)
	}
	if len(e.items) != 1 {
		t.Fatalf("expected 1 extracted item, got %d", len(e.items))
	}
	it := e.items[0]
	if !it.IsIdentityClosure {
		t.Fatalf("expected IsIdentityClosure=true even with InlineClosures=false; got false")
	}
	if it.ShouldInline {
		t.Fatalf("expected ShouldInline=false for an identity closure; got true")
	}
}

// TestIdentityClosureAssertOpDetected locks the type-assertion identity closure
// (OpAssert): a closure of the form `func(p any) T { return p.(T) }` must be
// recognized as an identity closure and collapse to the inline assertion `p.(T)`
// even when -inline (IIFE, Phase 3) is OFF. This covers the DI-common "narrow
// interface / any -> concrete type" wrapper that the original identity-closure
// analysis (only direct / addr / deref / convert) missed.
func TestIdentityClosureAssertOpDetected(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module testmod\n\ngo 1.21\n")
	writeTestFile(t, dir, "main/main.go", `package main

type T struct{}

var Closure = func(p any) T { return p.(T) }
`)

	cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}
	loaded, err := packages.Load(cfg, "testmod/main")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(loaded) == 0 || loaded[0].PkgPath == "" {
		t.Fatalf("main package not loaded")
	}
	if len(loaded[0].Errors) > 0 {
		t.Fatalf("load errors for %s: %v", loaded[0].PkgPath, loaded[0].Errors)
	}
	pkg := loaded[0]

	pkgMap := map[string]*packages.Package{pkg.PkgPath: pkg}
	am := NewAliasManager(pkg.PkgPath, alias.SimpleAliasStrategy{}, pkgMap, &logger.Logger{})
	am.LoadImportAliases()

	e := &Extractor{
		cfg:               &config.Config{InlineClosures: false}, // IIFE OFF — only identity should apply
		pkgMap:            pkgMap,
		mainPkgPath:       pkg.PkgPath,
		aliasManager:      am,
		globalProviderMap: make(map[string]int),
		typeStrCache:      make(map[types.Type]string),
	}

	funcLit := findClosureFuncLit(pkg, "Closure")
	if funcLit == nil {
		t.Fatalf("Closure funcLit not found")
	}

	if err := e.handleFuncLit(funcLit, pkg, false); err != nil {
		t.Fatalf("handleFuncLit: %v", err)
	}
	if len(e.items) != 1 {
		t.Fatalf("expected 1 extracted item, got %d", len(e.items))
	}
	it := e.items[0]
	if !it.IsIdentityClosure {
		t.Fatalf("expected IsIdentityClosure=true for type-assertion closure; got false")
	}
	if it.ShouldInline {
		t.Fatalf("expected ShouldInline=false for an identity closure; got true")
	}
	if it.IdentityOp != model.OpAssert {
		t.Fatalf("expected IdentityOp=%q for type-assertion closure, got %q", model.OpAssert, it.IdentityOp)
	}
	if it.IdentityTargetType == "" {
		t.Fatalf("expected non-empty IdentityTargetType for type-assertion closure")
	}
}

func findClosureFuncLit(pkg *packages.Package, name string) *ast.FuncLit {
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, n := range vs.Names {
					if n.Name != name || i >= len(vs.Values) {
						continue
					}
					if fl, ok := vs.Values[i].(*ast.FuncLit); ok {
						return fl
					}
				}
			}
		}
	}
	return nil
}

// TestCheckGenerationVisibilityVarScoping verifies that checkGenerationVisibility
// distinguishes package-level vars (which CAN be referenced cross-package and
// must be exported) from local variables / function / closure parameters (which
// are inlined into the generation target package and must never be flagged).
//
// Regression: a closure parameter named with an unexported identifier
// (e.g. dig.Invoke(func(f func() Config){ ... f() ... })) used to be misreported
// as `var "f" is private in package X and cannot be used from package Y`, even
// though f is a parameter of a function body that lives in the generated package.
func TestCheckGenerationVisibilityVarScoping(t *testing.T) {
	const mainPkgPath = "example/main"
	e := &Extractor{mainPkgPath: mainPkgPath}

	otherPkg := types.NewPackage("example/other", "other")
	curPkg := &packages.Package{Fset: token.NewFileSet()}

	// 1) Package-level unexported var in another package: MUST error.
	pkgScope := otherPkg.Scope()
	pkgVar := types.NewVar(token.NoPos, otherPkg, "privateVar", types.Typ[types.Int])
	pkgScope.Insert(pkgVar) // sets Parent() to the package scope
	if err := e.checkGenerationVisibility(pkgVar, curPkg); err == nil {
		t.Errorf("case 1: expected error for package-level unexported var referenced cross-package, got nil")
	}

	// 2) Function / closure parameter (nested scope): MUST be skipped.
	nested := types.NewScope(pkgScope, token.NoPos, token.NoPos, "func")
	paramVar := types.NewParam(token.NoPos, otherPkg, "privateVar", types.Typ[types.Int])
	nested.Insert(paramVar) // sets Parent() to the nested scope
	if err := e.checkGenerationVisibility(paramVar, curPkg); err != nil {
		t.Errorf("case 2: unexpected error for closure parameter (should be skipped): %v", err)
	}

	// 3) Local variable (nested scope): MUST be skipped.
	localVar := types.NewVar(token.NoPos, otherPkg, "privateVar", types.Typ[types.Int])
	nested.Insert(localVar)
	if err := e.checkGenerationVisibility(localVar, curPkg); err != nil {
		t.Errorf("case 3: unexpected error for local variable (should be skipped): %v", err)
	}
}
