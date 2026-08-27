package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/buildconstraint"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// checkContractVisibility is a pre-check that runs AFTER extraction but BEFORE
// any code is generated. It enforces the central digen DI contract:
//
//	A //go:build digen file (typically di.go) must contain ONLY the
//	dig.Build(...) wiring entry point — never the types, constructors, or
//	package-level variables the wiring references. Those must live in a file
//	WITHOUT the digen constraint (or in an imported package).
//
// Why this matters: the generated dig_gen.go carries //go:build !digen, so at a
// normal `go build` (without the digen tag) the digen file is EXCLUDED. If the
// wiring references a symbol defined inside that file, the generated code fails
// to compile with a confusing "undefined: X" error at the user's own build.
//
// Catching it here lets digen abort with a precise, actionable message instead
// of letting generation proceed and then surfacing the misleading
// "internal generator bug" the post-generation type-check net would otherwise
// report. (The net still double-checks as a backstop; see generator.go.)
func (e *Extractor) checkContractVisibility() error {
	digenObjs := e.collectDigenDefinedMainObjects()
	if len(digenObjs) == 0 {
		return nil
	}

	var violations []string
	seen := make(map[types.Object]bool)
	report := func(obj types.Object, pos token.Position, ctx string) {
		if obj == nil || seen[obj] {
			return
		}
		if _, ok := digenObjs[obj]; !ok {
			return
		}
		seen[obj] = true
		rel := e.relPath(pos.Filename)
		violations = append(violations, fmt.Sprintf(
			"  - %s %q (defined at %s) is referenced by the wiring (%s), but %s is a //go:build digen file. "+
				"At a normal `go build` (without the digen tag) that file is excluded, so the generated dig_gen.go cannot see %q.\n"+
				"    \U0001F4A1 Fix: move the definition of %q into a file WITHOUT the //go:build digen constraint (e.g. types.go), or into an imported package.",
			objKind(obj), obj.Name(), rel, ctx, rel, obj.Name(), obj.Name()))
	}

	for i := range e.items {
		it := &e.items[i]
		ctx := describeWiring(it)
		info := it.Pkg.TypesInfo

		// (1) Identifiers used by the option expression. Closures and dig.Supply
		//     keep the original expression in it.Expr (so we can see every
		//     identifier inside a closure body, or the supplied value). Local
		//     params/locals resolve to objects with a nil package and are
		//     naturally skipped.
		if it.Expr != nil {
			ast.Inspect(it.Expr, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj := info.ObjectOf(ident)
				if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == e.mainPkgPath {
					report(obj, it.Pkg.Fset.Position(ident.Pos()), ctx)
				}
				return true
			})
		}

		// (2) Non-closure constructor/invoker: handleProvide/handleInvoke do NOT
		//     stash the expression in it.Expr, so resolve the function object via
		//     the package scope and walk its full signature (params + results).
		//     This catches both the constructor itself and any main-package type
		//     it returns or consumes.
		if !it.IsClosure && it.FuncName != "" {
			if obj := it.Pkg.Types.Scope().Lookup(it.FuncName); obj != nil {
				if fn, ok := obj.(*types.Func); ok {
					report(obj, it.Pkg.Fset.Position(obj.Pos()), ctx)
					if sig, ok := fn.Type().(*types.Signature); ok {
						walkSignatureTypes(sig, e.mainPkgPath, func(t *types.Named) {
							report(t.Obj(), it.Pkg.Fset.Position(t.Obj().Pos()), ctx)
						})
					}
				}
			}
		}

		// (3) Closure: signature types (idents already covered by the it.Expr
		//     walk above) plus captured free-variable types.
		if it.IsClosure {
			if it.ClosureLit != nil {
				if sig, ok := it.Pkg.TypesInfo.TypeOf(it.ClosureLit).(*types.Signature); ok {
					walkSignatureTypes(sig, e.mainPkgPath, func(t *types.Named) {
						report(t.Obj(), it.Pkg.Fset.Position(t.Obj().Pos()), ctx)
					})
				}
			}
			for _, t := range it.FreeTypes {
				walkTypeMainPkgNamed(t, e.mainPkgPath, func(tn *types.Named) {
					report(tn.Obj(), it.Pkg.Fset.Position(tn.Obj().Pos()), ctx)
				})
			}
		}

		// (4) dig.Supply value type (the value itself is covered by (1); this
		//     catches its type when the value is, e.g., a main-package variable).
		if it.IsSupply && it.Expr != nil {
			if typ := it.Pkg.TypesInfo.TypeOf(it.Expr); typ != nil {
				walkTypeMainPkgNamed(typ, e.mainPkgPath, func(tn *types.Named) {
					report(tn.Obj(), it.Pkg.Fset.Position(tn.Obj().Pos()), ctx)
				})
			}
		}

		// (5) Merged parameter types (covers both closures and non-closures,
		//     including the params a non-closure constructor consumes).
		for _, p := range it.Params {
			if p.Type != nil {
				walkTypeMainPkgNamed(p.Type, e.mainPkgPath, func(tn *types.Named) {
					report(tn.Obj(), it.Pkg.Fset.Position(tn.Obj().Pos()), ctx)
				})
			}
		}
	}

	if len(violations) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("digen contract violation: the wiring references %d symbol(s) that are defined inside a //go:build digen file of package %s.\n", len(violations), e.mainPkgPath))
	b.WriteString("The generated dig_gen.go (which carries //go:build !digen) cannot see those symbols at a normal `go build`, so generation must stop before writing any file.\n")
	for _, v := range violations {
		b.WriteString(v)
		b.WriteString("\n")
	}
	return fmt.Errorf("%s", b.String())
}

// collectDigenDefinedMainObjects returns the set of package-level objects of the
// main package that are declared inside a //go:build digen file. These symbols
// are invisible to the generated (!digen) file, so any wiring that references
// them is a contract violation.
func (e *Extractor) collectDigenDefinedMainObjects() map[types.Object]token.Position {
	pkg, ok := e.pkgMap[e.mainPkgPath]
	if !ok || pkg.Types == nil || pkg.Fset == nil {
		return nil
	}
	files := make(map[string]*ast.File, len(pkg.Syntax))
	for _, f := range pkg.Syntax {
		files[pkg.Fset.Position(f.Package).Filename] = f
	}
	res := make(map[types.Object]token.Position)
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj == nil || obj.Pos() == token.NoPos {
			continue
		}
		// scope is the main package's scope, but guard anyway.
		if obj.Pkg() == nil || obj.Pkg().Path() != e.mainPkgPath {
			continue
		}
		fn := pkg.Fset.Position(obj.Pos()).Filename
		f, ok := files[fn]
		if !ok {
			continue
		}
		if buildconstraint.FileHasDigenConstraint(f) {
			res[obj] = pkg.Fset.Position(obj.Pos())
		}
	}
	return res
}

// walkTypeMainPkgNamed invokes visit for every *types.Named whose defining
// package is mainPkgPath. It recurses through pointers, slices, arrays, maps,
// channels, and generic type arguments, but deliberately does NOT descend into
// a type's Underlying() — that avoids infinite recursion on self-referential
// types (e.g. type Node struct { Next *Node }) while still catching the
// top-level Named type itself.
func walkTypeMainPkgNamed(t types.Type, mainPkgPath string, visit func(*types.Named)) {
	switch t := t.(type) {
	case *types.Named:
		if t.Obj() != nil && t.Obj().Pkg() != nil && t.Obj().Pkg().Path() == mainPkgPath {
			visit(t)
		}
		if args := t.TypeArgs(); args != nil {
			for i := 0; i < args.Len(); i++ {
				walkTypeMainPkgNamed(args.At(i), mainPkgPath, visit)
			}
		}
	case *types.Pointer:
		walkTypeMainPkgNamed(t.Elem(), mainPkgPath, visit)
	case *types.Slice:
		walkTypeMainPkgNamed(t.Elem(), mainPkgPath, visit)
	case *types.Array:
		walkTypeMainPkgNamed(t.Elem(), mainPkgPath, visit)
	case *types.Map:
		walkTypeMainPkgNamed(t.Key(), mainPkgPath, visit)
		walkTypeMainPkgNamed(t.Elem(), mainPkgPath, visit)
	case *types.Chan:
		walkTypeMainPkgNamed(t.Elem(), mainPkgPath, visit)
	}
}

// walkSignatureTypes invokes visit for every main-package Named type appearing
// in a function signature's parameters, results, and receiver.
func walkSignatureTypes(sig *types.Signature, mainPkgPath string, visit func(*types.Named)) {
	for i := 0; i < sig.Params().Len(); i++ {
		walkTypeMainPkgNamed(sig.Params().At(i).Type(), mainPkgPath, visit)
	}
	for i := 0; i < sig.Results().Len(); i++ {
		walkTypeMainPkgNamed(sig.Results().At(i).Type(), mainPkgPath, visit)
	}
	if recv := sig.Recv(); recv != nil {
		walkTypeMainPkgNamed(recv.Type(), mainPkgPath, visit)
	}
}

// objKind returns a human-readable noun for a referenced object.
func objKind(obj types.Object) string {
	switch obj.(type) {
	case *types.TypeName:
		return "type"
	case *types.Func:
		return "func"
	case *types.Var:
		return "var"
	case *types.Const:
		return "const"
	default:
		return "symbol"
	}
}

// describeWiring returns a short description of which wiring construct an item
// belongs to, for use in error messages.
func describeWiring(it *extractedItem) string {
	switch {
	case it.IsSupply:
		return "dig.Supply"
	case it.IsInvoke:
		if it.FuncName != "" {
			return "dig.Invoke(" + it.FuncName + ")"
		}
		return "dig.Invoke(closure)"
	case it.IsClosure:
		return "dig.Provide(closure)"
	default:
		if it.FuncName != "" {
			return "dig.Provide(" + it.FuncName + ")"
		}
		return "dig.Provide"
	}
}
