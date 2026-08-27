package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/generator"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/processor"

	"github.com/shanjunmei/dig/pkg/alias"
)

// TestRunPartialFailureWritesGoodKeepsBad exercises App.Run end-to-end on a
// throwaway module with one valid and one invalid dig.Build package. It asserts:
//   - Run returns a non-zero signal (an error) when ANY package failed, so CI
//     cannot silently go green while some packages were not generated;
//   - the good package's dig_gen.go is still written;
//   - the bad package's dig_gen.go is NOT written (its failure aborts only its
//     own generation).
func TestRunPartialFailureWritesGoodKeepsBad(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	cfg := &config.Config{Paths: []string{"./pkg_good", "./pkg_bad"}}
	ld := loader.NewPackageLoader()
	log := logger.NewLogger(cfg)
	gen := generator.NewGenerator(log, cfg)
	p := processor.NewProcessor(ld, gen, log, cfg)
	a := NewApp(p, ld, log, alias.NewAliasStrategy(alias.AliasFull), cfg)

	err = a.Run()
	if err == nil {
		t.Fatal("expected Run to fail when one of the packages failed to generate")
	}
	if !strings.Contains(err.Error(), "failed to generate") {
		t.Fatalf("error should mention failed generation, got: %v", err)
	}
	// The summary line must be the LAST thing a user sees even on failure, so a
	// long scroll cannot hide the fact that 1 package did succeed.
	lines := strings.Split(strings.TrimSpace(err.Error()), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "[digen] generated 1/2 packages (1 failed), cost:") {
		t.Fatalf("summary line should be the final output, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pkg_good", "dig_gen.go")); statErr != nil {
		t.Fatalf("good package dig_gen.go should have been written: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pkg_bad", "dig_gen.go")); !os.IsNotExist(statErr) {
		t.Fatalf("bad package dig_gen.go must NOT be written, stat err: %v", statErr)
	}
}

// TestRunAllFailedNonZeroExit verifies that when every dig.Build package fails,
// Run reports the total count with an actionable message rather than a bare
// "no packages" error.
func TestRunAllFailedNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	cfg := &config.Config{Paths: []string{"./pkg_bad"}}
	ld := loader.NewPackageLoader()
	log := logger.NewLogger(cfg)
	gen := generator.NewGenerator(log, cfg)
	p := processor.NewProcessor(ld, gen, log, cfg)
	a := NewApp(p, ld, log, alias.NewAliasStrategy(alias.AliasFull), cfg)

	err = a.Run()
	if err == nil {
		t.Fatal("expected Run to fail when the only dig.Build package failed")
	}
	if !strings.Contains(err.Error(), "failed to generate") {
		t.Fatalf("error should report failed generation, got: %v", err)
	}
	if !strings.Contains(err.Error(), "(0 succeeded)") {
		t.Fatalf("error should state zero packages succeeded, got: %v", err)
	}
	// Even a total failure must end with the summary line so the tail of the
	// output reflects reality (0 succeeded) rather than a bare error list.
	lines := strings.Split(strings.TrimSpace(err.Error()), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "[digen] generated 0/1 packages (1 failed), cost:") {
		t.Fatalf("summary line should be the final output on total failure, got: %v", err)
	}
}

// TestRunGoGenerateDirectiveStaysRelative guards against the //go:generate
// -out flag regressing to an absolute path. In non-"." mode the resolved output
// path is absolute (dir/dig_gen.go), but the directive embedded in the
// generated file must keep the bare file name so `go generate` works from any
// checkout — an absolute path would bake the machine's directory layout into
// the committed generated file.
func TestRunGoGenerateDirectiveStaysRelative(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	cfg := &config.Config{Paths: []string{"./pkg_good"}}
	ld := loader.NewPackageLoader()
	log := logger.NewLogger(cfg)
	gen := generator.NewGenerator(log, cfg)
	p := processor.NewProcessor(ld, gen, log, cfg)
	a := NewApp(p, ld, log, alias.NewAliasStrategy(alias.AliasFull), cfg)

	if err := a.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "pkg_good", "dig_gen.go"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(line, "go:generate") {
			continue
		}
		if !strings.Contains(line, "-out=dig_gen.go") {
			t.Fatalf("go:generate -out must be the relative file name, got: %q", line)
		}
		if strings.Contains(line, "\\") || strings.Contains(line, ":/") {
			t.Fatalf("go:generate -out must not be an absolute path, got: %q", line)
		}
		return
	}
	t.Fatal("generated file has no go:generate directive")
}

// writeTestModule lays out a temporary module with a valid dig.Build package
// (pkg_good) and an invalid one (pkg_bad, an unconsumable provider type). The
// module replaces github.com/shanjunmei/dig with the repository checked out
// locally. No repository files are touched.
func writeTestModule(t *testing.T, dir string) {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/diapp_test\n\ngo 1.25.0\n\nrequire github.com/shanjunmei/dig v0.0.0\n\nreplace github.com/shanjunmei/dig => " + filepath.ToSlash(repo) + "\n",
		"pkg_good/types.go": "package pkg_good\n\ntype Svc struct{}\n\nfunc NewSvc() *Svc { return &Svc{} }\n",
		"pkg_good/di.go": "//go:build digen\n\npackage pkg_good\n\nimport (\n\t\"context\"\n\n\t\"github.com/shanjunmei/dig\"\n)\n\nfunc InitApp() func(context.Context) error {\n\treturn dig.Build(\n\t\tdig.Provide(NewSvc),\n\t\tdig.Invoke(func(s *Svc) { _ = s }),\n\t)\n}\n",
		"pkg_bad/types.go": "package pkg_bad\n",
		"pkg_bad/di.go": "//go:build digen\n\npackage pkg_bad\n\nimport (\n\t\"context\"\n\n\t\"github.com/shanjunmei/dig\"\n)\n\nfunc InitApp() func(context.Context) error {\n\treturn dig.Build(\n\t\tdig.Invoke(func(missing *struct{}) { _ = missing }),\n\t)\n}\n",
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
}

// TestNewApp verifies the dependency wiring (loader -> generator -> processor ->
// app) constructs without panicking and yields a usable App.
func TestNewApp(t *testing.T) {
	cfg := &config.Config{}
	ld := loader.NewPackageLoader()
	log := logger.NewLogger(cfg)
	gen := generator.NewGenerator(log, cfg)
	p := processor.NewProcessor(ld, gen, log, cfg)
	strat := alias.NewAliasStrategy(alias.AliasFull)

	a := NewApp(p, ld, log, strat, cfg)
	if a == nil {
		t.Fatal("NewApp returned nil")
	}
}
