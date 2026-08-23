package loader

import (
	"errors"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestCollectAllPackages covers the recursive package collector: it must visit
// every reachable package exactly once (terminating on import cycles) and skip
// packages with an empty PkgPath.
func TestCollectAllPackages(t *testing.T) {
	root := &packages.Package{PkgPath: "root"}
	a := &packages.Package{PkgPath: "a"}
	b := &packages.Package{PkgPath: "b"}
	root.Imports = map[string]*packages.Package{"a": a, "b": b}
	a.Imports = map[string]*packages.Package{"b": b}
	b.Imports = map[string]*packages.Package{"a": a} // cycle back to a

	got := collectAllPackages([]*packages.Package{root})
	if len(got) != 3 {
		t.Fatalf("collectAllPackages collected %d, want 3", len(got))
	}
	for _, p := range []string{"root", "a", "b"} {
		if _, ok := got[p]; !ok {
			t.Fatalf("missing package %q in collected set", p)
		}
	}

	// Empty PkgPath packages (e.g. ambiguous imports / errors) must be skipped.
	if got := collectAllPackages([]*packages.Package{{PkgPath: ""}}); len(got) != 0 {
		t.Fatalf("empty PkgPath package should be skipped, got %d", len(got))
	}
}

// TestFindInjectorFunctionsNoBuildIsSentinel verifies a package without a
// dig.Build call yields the typed ErrNoDigBuildCall rather than a stringly
// error, so callers can match it with errors.Is.
func TestFindInjectorFunctionsNoBuildIsSentinel(t *testing.T) {
	ld := NewPackageLoader()
	pkgs, _, err := ld.Load([]string{"github.com/shanjunmei/dig/internal/loader"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var pkg *packages.Package
	for _, p := range pkgs {
		if p.PkgPath == "github.com/shanjunmei/dig/internal/loader" {
			pkg = p
		}
	}
	if pkg == nil {
		t.Fatal("loader package not loaded")
	}
	if _, err := FindInjectorFunctions(pkg); !errors.Is(err, ErrNoDigBuildCall) {
		t.Fatalf("expected ErrNoDigBuildCall, got %v", err)
	}
}

// TestFindInjectorFunctionsPositive verifies a real dig.Build entry point is
// located in an example package.
func TestFindInjectorFunctionsPositive(t *testing.T) {
	ld := NewPackageLoader()
	pkgs, _, err := ld.Load([]string{"github.com/shanjunmei/dig/example/app_basic"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var pkg *packages.Package
	for _, p := range pkgs {
		if p.PkgPath == "github.com/shanjunmei/dig/example/app_basic" {
			pkg = p
		}
	}
	if pkg == nil {
		t.Fatal("example/app_basic not loaded")
	}
	target, err := FindInjectorFunctions(pkg)
	if err != nil {
		t.Fatalf("FindInjectorFunctions: %v", err)
	}
	if target == nil || target.Node == nil {
		t.Fatal("expected a non-nil GenTarget with a resolved function node")
	}
}
