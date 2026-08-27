package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/model"
)

// TestVerifyGeneratedBatch proves the batched type-check net attributes errors
// per artifact: a generated file that type-checks cleanly is absent from the
// result map, one that fails to type-check yields an actionable error under its
// PkgPath. Both artifacts are injected via a single Overlay load.
func TestVerifyGeneratedBatch(t *testing.T) {
	dir := writeTestModule(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	cfg := &config.Config{}
	log := logger.NewLogger(cfg)
	g := NewGenerator(log, cfg)

	ld := loader.NewPackageLoader()
	pkgs, _, err := ld.Load([]string{"./pkg_good"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no package loaded")
	}
	pkg := pkgs[0]
	genFile := filepath.Join(dir, "pkg_good", "dig_gen.go")

	good := GeneratedArtifact{
		Pkg:     pkg,
		File:    genFile,
		Content: []byte("package pkg_good\n\nfunc ok() {}\n"),
	}
	bad := GeneratedArtifact{
		Pkg:     pkg,
		File:    genFile,
		Content: []byte("package pkg_good\n\nfunc bad() { undefinedSymbol() }\n"),
	}

	if errs := g.VerifyGeneratedBatch([]GeneratedArtifact{bad}); errs[pkg.PkgPath] == nil {
		t.Fatal("expected a type-check error for the bad artifact, got nil")
	}
	if errs := g.VerifyGeneratedBatch([]GeneratedArtifact{good}); errs[pkg.PkgPath] != nil {
		t.Fatalf("expected no type-check error for the good artifact, got: %v", errs[pkg.PkgPath])
	}
	if errs := g.VerifyGeneratedBatch(nil); errs != nil {
		t.Fatalf("expected nil for empty input, got: %v", errs)
	}
}

// writeTestModule lays out a throwaway module with a package that has a valid
// dig.Build entry point but no generated file yet, so tests can inject one via
// an Overlay. The module replaces github.com/shanjunmei/dig with the repository
// checked out locally (found via ../.. relative to this test package).
func writeTestModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/digen_test\n\ngo 1.25.0\n\nrequire github.com/shanjunmei/dig v0.0.0\n\nreplace github.com/shanjunmei/dig => " + filepath.ToSlash(repo) + "\n",
		"pkg_good/types.go": "package pkg_good\n\ntype Svc struct{}\n\nfunc NewSvc() *Svc { return &Svc{} }\n",
		"pkg_good/di.go": "//go:build digen\n\npackage pkg_good\n\nimport (\n\t\"context\"\n\n\t\"github.com/shanjunmei/dig\"\n)\n\nfunc InitApp() func(context.Context) error {\n\treturn dig.Build(\n\t\tdig.Provide(NewSvc),\n\t\tdig.Invoke(func(s *Svc) { _ = s }),\n\t)\n}\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBuildIIFECall(t *testing.T) {
	// Not inline or empty def -> empty string.
	if got := buildIIFECall(model.Node{ShouldInline: false, ClosureDef: "func f() int { return 1 }"}); got != "" {
		t.Fatalf("expected empty when ShouldInline=false, got %q", got)
	}
	if got := buildIIFECall(model.Node{ShouldInline: true, ClosureDef: ""}); got != "" {
		t.Fatalf("expected empty when ClosureDef empty, got %q", got)
	}

	// Strips the function name, turning a named func into an IIFE.
	in := "func dv_foo(a int, b string) *T {\n\treturn newT(a, b)\n}"
	want := "func(a int, b string) *T {\n\treturn newT(a, b)\n}"
	if got := buildIIFECall(model.Node{ShouldInline: true, ClosureDef: in}); got != want {
		t.Fatalf("IIFE conversion:\n got: %q\nwant: %q", got, want)
	}

	// Defensive: no opening paren -> returned unchanged.
	if got := buildIIFECall(model.Node{ShouldInline: true, ClosureDef: "func foo"}); got != "func foo" {
		t.Fatalf("expected def unchanged without paren, got %q", got)
	}
}

func TestBuildIdentityConversion(t *testing.T) {
	// Not an identity closure -> empty.
	if got := buildIdentityConversion(model.Node{IsIdentityClosure: false, IdentityOp: model.OpDirect, IdentityTargetType: "T"}, "x"); got != "" {
		t.Fatalf("expected empty when IsIdentityClosure=false, got %q", got)
	}

	cases := []struct {
		op   model.OpKind
		want string
	}{
		{model.OpDirect, "T(x)"},
		{model.OpAddr, "&x"},
		{model.OpDeref, "*x"},
		{model.OpConvert, "T(x)"},
		{model.OpAssert, "x.(T)"},
	}
	for _, c := range cases {
		got := buildIdentityConversion(model.Node{IsIdentityClosure: true, IdentityOp: c.op, IdentityTargetType: "T"}, "x")
		if got != c.want {
			t.Fatalf("op=%s: got %q want %q", c.op, got, c.want)
		}
	}

	// Unknown op -> empty (safe no-op).
	if got := buildIdentityConversion(model.Node{IsIdentityClosure: true, IdentityOp: "bogus", IdentityTargetType: "T"}, "x"); got != "" {
		t.Fatalf("expected empty for unknown op, got %q", got)
	}
}
