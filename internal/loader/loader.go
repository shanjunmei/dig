package loader

import (
	"errors"
	"fmt"
	"go/ast"
	"strings"

	"github.com/shanjunmei/dig/internal/extractor"
	"github.com/shanjunmei/dig/internal/model"
	"golang.org/x/tools/go/packages"
)

const tagBuild = model.TagBuild

// ErrNoDigBuildCall indicates a package has no function containing a dig.Build
// call. It lets callers distinguish "this package is not a DI entry point" from
// real extraction errors without fragile string matching.
var ErrNoDigBuildCall = errors.New("no function containing dig.Build call found")

// 为了简化，我们不在此处导入 logger，而是将日志功能通过参数传递，或直接在函数内使用全局？
// 更好的方式是让 loader 不依赖 logger，而是由上层处理错误和日志。
// 但原代码中 loadPackages 内有 debugf，我们可将其改为返回错误或由调用者打印。
// 我们可以让 loader 持有 logger，但需要避免循环导入（loader 导入 logger，logger 导入 config）。
// 这里我们直接让 loader 不依赖 logger，由 processor 打印。
// 但为了保持一致，我们让 loader 接受一个日志函数。
// 我决定让 loader 不依赖 logger，而由 processor 处理。

type PackageLoader struct{}

func NewPackageLoader() *PackageLoader {
	return &PackageLoader{}
}

func (l *PackageLoader) Load(paths []string) ([]*packages.Package, map[string]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedName |
			packages.NeedModule | packages.NeedFiles | packages.NeedTypesInfo |
			packages.NeedImports | packages.NeedDeps,
		Tests:      false,
		BuildFlags: []string{"-tags=" + tagBuild},
	}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, nil, fmt.Errorf("packages.Load failed: %w%s", err, hintForLoadFailure(paths, err.Error()))
	}
	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no packages loaded")
	}

	pkgMap := collectAllPackages(pkgs)

	var errs []string
	for _, p := range pkgMap {
		if len(p.Errors) > 0 {
			for _, e := range p.Errors {
				errs = append(errs, fmt.Sprintf("package %s: %v", p.PkgPath, e))
			}
		}
	}
	if len(errs) > 0 {
		joined := strings.Join(errs, "\n")
		return nil, nil, fmt.Errorf("compilation errors found in packages:\n%s%s", joined, hintForLoadFailure(paths, joined))
	}

	return pkgs, pkgMap, nil
}

// hintForLoadFailure appends an actionable hint when a package pattern could not
// be resolved. The raw go toolchain message (e.g. "package example/app is not in
// std") is not actionable on its own — users often omit the ./ prefix for a
// directory or mistype an import path. The hint explains the two accepted forms
// so the fix is obvious. Only triggers on resolution-style failures to avoid
// noise on unrelated errors (e.g. missing go.mod).
func hintForLoadFailure(paths []string, errMsg string) string {
	lower := strings.ToLower(errMsg)
	unresolved := strings.Contains(lower, "is not in std") ||
		strings.Contains(lower, "cannot find package") ||
		strings.Contains(lower, "no required module provides package") ||
		strings.Contains(lower, "not in goroot") ||
		strings.Contains(lower, "no such package") ||
		strings.Contains(lower, "malformed import path") ||
		strings.Contains(lower, "directory not found") ||
		strings.Contains(lower, "no such file or directory")
	if !unresolved {
		return ""
	}
	return "\n\n  💡 digen accepts Go package patterns. To target a directory, prefix it with ./ " +
		"(e.g. `digen ./example/app` or `digen ./...`); to target an import path, use the full " +
		"module path (e.g. `digen github.com/you/module/pkg`)."
}

func collectAllPackages(rootPkgs []*packages.Package) map[string]*packages.Package {
	pkgMap := make(map[string]*packages.Package)
	var collect func(*packages.Package)
	collect = func(p *packages.Package) {
		if _, exists := pkgMap[p.PkgPath]; exists {
			return
		}
		if p.PkgPath == "" {
			return
		}
		pkgMap[p.PkgPath] = p
		for _, impPkg := range p.Imports {
			collect(impPkg)
		}
	}
	for _, rootPkg := range rootPkgs {
		collect(rootPkg)
	}
	return pkgMap
}

// FindInjectorFunctions 查找包含 dig.Build 的函数
func FindInjectorFunctions(pkg *packages.Package) (*model.GenTarget, error) {
	var targets []model.GenTarget
	for idx, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			fnDecl, ok := decl.(*ast.FuncDecl)
			if !ok || fnDecl.Body == nil {
				continue
			}
			if extractor.FindDigCallInBlock(fnDecl.Body, pkg.TypesInfo, "Build") == nil {
				continue
			}
			if err := extractor.ValidateReturnType(fnDecl, pkg.TypesInfo, pkg.Fset); err != nil {
				return nil, err
			}
			targets = append(targets, model.GenTarget{
				FuncName: fnDecl.Name.Name,
				Node:     fnDecl,
				File:     pkg.GoFiles[idx],
			})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: in package %s\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error", ErrNoDigBuildCall, pkg.PkgPath)
	}
	if len(targets) > 1 {
		var locations []string
		for _, t := range targets {
			pos := pkg.Fset.Position(t.Node.Pos())
			locations = append(locations, fmt.Sprintf("  %s at %s", t.FuncName, pos))
		}
		return nil, fmt.Errorf("multiple functions containing dig.Build call found (only one allowed):\n%s\n  💡 Fix: keep exactly one function with dig.Build per package", strings.Join(locations, "\n"))
	}
	return &targets[0], nil
}
