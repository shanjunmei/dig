package extractor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/shanjunmei/dig/internal/model"
)

// typeInfoForClosure parses src (a package without imports), type-checks it and
// returns (fset, info, funcLits) where funcLits are the *ast.FuncLit nodes found
// in declaration order.
func typeInfoForClosure(t *testing.T, src string) (*token.FileSet, *types.Info, []*ast.FuncLit) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: nil}
	if _, err := conf.Check("test", fset, []*ast.File{f}, info); err != nil {
		t.Fatalf("type-check: %v", err)
	}
	var lits []*ast.FuncLit
	ast.Inspect(f, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok && fl.Type.Params != nil {
			lits = append(lits, fl)
		}
		return true
	})
	return fset, info, lits
}

// TestAnalyzeIdentityClosure_FunctionCallNotCollapsed verifies that a closure
// whose body returns a function call — e.g.
//
//	func(st *Store) *LookupTool { return NewLookupTool(st) }
//
// is NOT treated as an identity conversion. Collapsing it would emit
// `LookupTool(dvN)` and fail to compile (regression for hermes
// `tool.ArchiveLookupTool` provider).
func TestAnalyzeIdentityClosure_FunctionCallNotCollapsed(t *testing.T) {
	src := `package test

type Store struct{}

type LookupTool struct{}

func NewLookupTool(s *Store) *LookupTool { return &LookupTool{} }

var f1 = func(s *Store) *LookupTool { return NewLookupTool(s) }
var f2 = func(s Store) LookupTool { return LookupTool(s) }
var f3 = func(s *Store) *Store { return s }
`
	_, info, lits := typeInfoForClosure(t, src)
	if len(lits) != 3 {
		t.Fatalf("expected 3 func literals, got %d", len(lits))
	}

	// f1: return NewLookupTool(s) — a function call, NOT an identity conversion.
	if retExpr, op := analyzeIdentityClosure(lits[0], nil, info); retExpr != nil {
		t.Fatalf("function-call closure should NOT be an identity conversion, got op=%q", op)
	}

	// f2: return LookupTool(s) — a real type conversion, still collapsed.
	if retExpr, op := analyzeIdentityClosure(lits[1], nil, info); retExpr == nil {
		t.Fatalf("type-conversion closure should be an identity conversion, got nil")
	} else if op != model.OpConvert {
		t.Fatalf("expected OpConvert, got %q", op)
	}

	// f3: return s — direct pass-through, still collapsed.
	if retExpr, op := analyzeIdentityClosure(lits[2], nil, info); retExpr == nil {
		t.Fatalf("direct-return closure should be an identity conversion, got nil")
	} else if op != model.OpDirect {
		t.Fatalf("expected OpDirect, got %q", op)
	}
}

// TestAnalyzeIdentityClosure_CrossPkgFunctionCall ensures a qualified function
// call (pkg.NewFoo(x)) is also not collapsed, while a qualified conversion
// (pkg.T(x)) is.
func TestAnalyzeIdentityClosure_CrossPkgFunctionCallNotCollapsed(t *testing.T) {
	src := `package test

type Other struct{}
type Alias struct{}

func NewAlias(x *Other) *Alias { return &Alias{} }

var f1 = func(x *Other) *Alias { return NewAlias(x) }
`
	_, info, lits := typeInfoForClosure(t, src)
	if len(lits) != 1 {
		t.Fatalf("expected 1 func literal, got %d", len(lits))
	}
	// NewAlias resolves to *types.Func → must NOT be collapsed into an OpConvert.
	if retExpr, op := analyzeIdentityClosure(lits[0], nil, info); retExpr != nil {
		t.Fatalf("qualified function call should NOT be an identity conversion, got op=%q", op)
	}
}

// TestAnalyzeIdentityClosure_ConvertTargetUsesFunType verifies that for a
// conversion whose result type differs from the conversion target (here the
// declared return type is interface I, the target is concrete B), the returned
// target expression is the Fun (B), NOT the return type (I). Emitting I(x)
// instead of B(x) would fail to compile when the parameter type does not
// implement I.
func TestAnalyzeIdentityClosure_ConvertTargetUsesFunType(t *testing.T) {
	src := `package test

type A struct{}
type B struct{}
type I interface{ M() }

func (B) M() {}

var f1 = func(p A) I { return B(p) }
`
	_, info, lits := typeInfoForClosure(t, src)
	if len(lits) != 1 {
		t.Fatalf("expected 1 func literal, got %d", len(lits))
	}
	retExpr, op := analyzeIdentityClosure(lits[0], nil, info)
	if retExpr == nil {
		t.Fatalf("type-conversion closure should be an identity conversion, got nil")
	}
	if op != model.OpConvert {
		t.Fatalf("expected OpConvert, got %q", op)
	}
	ident, ok := retExpr.(*ast.Ident)
	if !ok || ident.Name != "B" {
		t.Fatalf("expected target expression to be Fun (B), got %T %v", retExpr, retExpr)
	}
}

// TestAnalyzeIdentityClosure_ParenConvert verifies parenthesized conversions
// (T)(x) are still recognized as conversions.
func TestAnalyzeIdentityClosure_ParenConvert(t *testing.T) {
	src := `package test

type A struct{}
type B struct{}

var f1 = func(p A) B { return (B)(p) }
`
	_, info, lits := typeInfoForClosure(t, src)
	if len(lits) != 1 {
		t.Fatalf("expected 1 func literal, got %d", len(lits))
	}
	retExpr, op := analyzeIdentityClosure(lits[0], nil, info)
	if retExpr == nil {
		t.Fatalf("parenthesized conversion should be an identity conversion, got nil")
	}
	if op != model.OpConvert {
		t.Fatalf("expected OpConvert, got %q", op)
	}
}

// TestAnalyzeIdentityClosure_NilTypeInfoConservative verifies that when TypesInfo
// is unavailable the analyzer does NOT collapse a function-call closure (the
// previous behavior guessed OpConvert and produced broken conversions).
func TestAnalyzeIdentityClosure_NilTypeInfoConservative(t *testing.T) {
	src := `package test

type Store struct{}
type LookupTool struct{}

func NewLookupTool(s *Store) *LookupTool { return &LookupTool{} }

var f1 = func(s *Store) *LookupTool { return NewLookupTool(s) }
`
	_, info, lits := typeInfoForClosure(t, src)
	if len(lits) != 1 {
		t.Fatalf("expected 1 func literal, got %d", len(lits))
	}
	// With real type info the function call is correctly NOT collapsed.
	if retExpr, op := analyzeIdentityClosure(lits[0], nil, info); retExpr != nil {
		t.Fatalf("function-call closure should NOT be an identity conversion, got op=%q", op)
	}
	// With nil type info the analyzer must be conservative and NOT collapse either.
	if retExpr, op := analyzeIdentityClosure(lits[0], nil, nil); retExpr != nil {
		t.Fatalf("nil-TypeInfo must not collapse function-call closure, got op=%q", op)
	}
}
