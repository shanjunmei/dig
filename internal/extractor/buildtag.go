package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/buildconstraint"
	"github.com/shanjunmei/dig/internal/model"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/packages"
)

func findFuncDecl(pkg *packages.Package, name string) *ast.FuncDecl {
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if ok && fd.Name.Name == name {
				return fd
			}
		}
	}
	return nil
}

func (e *Extractor) checkBuildSourceConstraint() error {
	pkg, ok := e.pkgMap[e.mainPkgPath]
	if !ok {
		return nil
	}
	for _, f := range pkg.Syntax {
		buildCall := findDigBuildCallInFile(f, pkg.TypesInfo)
		if buildCall == nil {
			continue
		}
		if buildconstraint.FileHasDigenConstraint(f) {
			continue
		}
		pos := pkg.Fset.Position(buildCall.Pos())
		rel := e.relPath(pos.Filename)
		return fmt.Errorf("at %s: file contains dig.Build(...) but is missing the `//go:build digen` build constraint. "+
			"At a normal `go build` (without the digen tag) both this file and the generated dig_gen.go (which carries `//go:build !digen`) would be compiled, "+
			"defining InitApp twice and causing a \"InitApp redeclared\" error. "+
			"The generated file is excluded only when this file is built WITH the digen tag.\n"+
			"  💡 Fix: add `//go:build digen` to the top of %s (before the package clause)",
			pos, rel)
	}
	return nil
}

func findDigBuildCallInFile(f *ast.File, info *types.Info) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		obj := info.ObjectOf(sel.Sel)
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == diPkgPath && obj.Name() == "Build" {
			found = call
			return false
		}
		return true
	})
	return found
}

func (e *Extractor) BuildFinalNodes() ([]model.Node, error) {
	// A source file containing dig.Build(...) must carry `//go:build digen`,
	// otherwise the user hits a confusing "InitApp redeclared" error at their
	// own build. Catch it here so generation aborts (no file is written).
	if err := e.checkBuildSourceConstraint(); err != nil {
		return nil, err
	}
	e.populateUsedPkgs()
	nodes, err := e.buildFinalNodes()
	if err != nil {
		return nil, err
	}
	// Pre-check the digen DI contract: abort with a clear message if the wiring
	// references any symbol defined inside a //go:build digen file of the main
	// package. Runs before any code is written. See contract.go.
	if err := e.checkContractVisibility(); err != nil {
		return nil, err
	}
	return nodes, nil
}
