package processor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/generator"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/alias"
	"golang.org/x/tools/go/packages"
)

const cacheTestPkg = "github.com/shanjunmei/dig/example/app_basic"

func setupCacheTest(t *testing.T) (*Processor, *packages.Package, *model.GenTarget, map[string]*packages.Package, alias.AliasStrategy) {
	t.Helper()
	cfg := &config.Config{
		AliasType:    string(alias.AliasFull),
		TypeCheckNet: false,
		Cache:        true,
		CacheDir:     t.TempDir(),
	}
	ld := loader.NewPackageLoader()
	pkgs, pkgMap, err := ld.Load([]string{cacheTestPkg})
	if err != nil {
		t.Fatalf("load %s: %v", cacheTestPkg, err)
	}
	var pkg *packages.Package
	for _, p := range pkgs {
		if p.PkgPath == cacheTestPkg {
			pkg = p
		}
	}
	if pkg == nil {
		t.Fatalf("package %s not in loaded set", cacheTestPkg)
	}
	target, err := loader.FindInjectorFunctions(pkg)
	if err != nil {
		t.Fatalf("find injector: %v", err)
	}
	strategy := alias.NewAliasStrategy(alias.AliasType(cfg.AliasType))
	log := logger.NewLogger(cfg)
	gen := generator.NewGenerator(log, cfg)
	p := NewProcessor(nil, gen, log, cfg)
	return p, pkg, target, pkgMap, strategy
}

// generateToFile runs the real generator on a node set and returns the emitted
// source so two extractions can be compared byte-for-byte — the exact way the
// cache would be observed by a downstream consumer.
func generateToFile(t *testing.T, g *generator.Generator, pkg *packages.Package, target *model.GenTarget, nodes []model.Node, im, pm, nm map[string]string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "dig_gen.go")
	tgt := *target
	tgt.File = out
	refCount := make(map[string]int)
	for _, n := range nodes {
		for _, a := range n.Args {
			refCount[a.Name]++
		}
	}
	if err := g.WriteGeneratedCode(pkg, &tgt, nodes, refCount, im, pm, nm, pkg.Fset); err != nil {
		t.Fatalf("write generated code: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated: %v", err)
	}
	return string(b)
}

func TestProcessorCacheHitProducesIdenticalIR(t *testing.T) {
	p, pkg, target, pkgMap, strategy := setupCacheTest(t)

	// Fresh extraction (the cache-off path).
	freshNodes, fIm, fPm, fNm, err := p.extractAndBuildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// First buildNodes call with caching enabled: miss -> populate cache.
	cached1Nodes, c1Im, c1Pm, c1Nm, err := p.buildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		t.Fatalf("buildNodes (populate): %v", err)
	}
	// Second call: hit.
	cached2Nodes, c2Im, c2Pm, c2Nm, err := p.buildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		t.Fatalf("buildNodes (hit): %v", err)
	}

	g := generator.NewGenerator(logger.NewLogger(p.cfg), p.cfg)
	freshCode := generateToFile(t, g, pkg, target, freshNodes, fIm, fPm, fNm)
	code1 := generateToFile(t, g, pkg, target, cached1Nodes, c1Im, c1Pm, c1Nm)
	code2 := generateToFile(t, g, pkg, target, cached2Nodes, c2Im, c2Pm, c2Nm)

	if code1 != code2 {
		t.Fatalf("cache hit produced different code than cache miss:\n--- miss ---\n%s\n--- hit ---\n%s", code1, code2)
	}
	if freshCode != code1 {
		t.Fatalf("cached IR diverges from fresh extraction:\n--- fresh ---\n%s\n--- cached ---\n%s", freshCode, code1)
	}

	// The cached IR must survive the JSON round-trip intact. The only allowed
	// difference is the schema-version marker that serialization stamps on,
	// which the generator ignores.
	assertNodesEquivalent(t, freshNodes, cached2Nodes)
}

func assertNodesEquivalent(t *testing.T, a, b []model.Node) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("node count mismatch: %d vs %d", len(a), len(b))
	}
	norm := func(ns []model.Node) []model.Node {
		out := make([]model.Node, len(ns))
		for i, n := range ns {
			n.SchemaVer = 0 // serialization stamps this; not part of the IR payload
			out[i] = n
		}
		return out
	}
	if !reflect.DeepEqual(norm(a), norm(b)) {
		t.Fatalf("nodes differ after normalizing schema version:\n%#v\nvs\n%#v", a, b)
	}
}

// TestCacheKeyIncludesDependencyChanges proves the cache key is sensitive to a
// dependency's source content (so a dep API change invalidates the cache) and
// is deterministic (restoring the source restores the key). It builds an
// in-memory package graph so it needs no real module on disk.
func TestCacheKeyIncludesDependencyChanges(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.go")
	depFile := filepath.Join(dir, "dep.go")
	mustWrite(t, mainFile, "package main\n")
	mustWrite(t, depFile, "package dep\n")

	dep := &packages.Package{PkgPath: "example.com/dep", GoFiles: []string{depFile}}
	main := &packages.Package{
		PkgPath: "example.com/main",
		GoFiles: []string{mainFile},
		Imports: map[string]*packages.Package{"example.com/dep": dep},
	}
	p := &Processor{cfg: &config.Config{AliasType: "full", InlineClosures: false}}

	key1, err := p.cacheKey(main)
	if err != nil {
		t.Fatalf("cacheKey: %v", err)
	}

	// A dependency's source change must change the key.
	mustWrite(t, depFile, "package dep\n\nfunc Changed() {}\n")
	key2, err := p.cacheKey(main)
	if err != nil {
		t.Fatalf("cacheKey after dep change: %v", err)
	}
	if key1 == key2 {
		t.Fatal("cache key did not change when a dependency's source changed")
	}

	// Restoring the dependency must restore the key (deterministic).
	mustWrite(t, depFile, "package dep\n")
	key3, err := p.cacheKey(main)
	if err != nil {
		t.Fatalf("cacheKey after dep restore: %v", err)
	}
	if key3 != key1 {
		t.Fatalf("cache key not deterministic: %s vs %s", key1, key3)
	}

	// The package's own source change must also change the key.
	mustWrite(t, mainFile, "package main\n\nfunc Own() {}\n")
	key4, err := p.cacheKey(main)
	if err != nil {
		t.Fatalf("cacheKey after own change: %v", err)
	}
	if key4 == key1 {
		t.Fatal("cache key did not change when the package's own source changed")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
