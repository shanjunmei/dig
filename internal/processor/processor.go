package processor

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/extractor"
	"github.com/shanjunmei/dig/internal/generator"
	"github.com/shanjunmei/dig/internal/ir"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/model"

	"github.com/shanjunmei/dig/pkg/alias"
	"golang.org/x/tools/go/packages"
)

type Processor struct {
	loader    *loader.PackageLoader
	generator *generator.Generator
	logger    *logger.Logger
	cfg       *config.Config
}

func NewProcessor(loader *loader.PackageLoader, generator *generator.Generator, logger *logger.Logger, cfg *config.Config) *Processor {
	return &Processor{
		loader:    loader,
		generator: generator,
		logger:    logger,
		cfg:       cfg,
	}
}

// Artifact is a package's freshly generated file held in memory, pending the
// batch type-check net and the final write. Returning it (instead of writing
// inside Process) lets App.Run verify every generated file with a single
// packages.Load pass and skip writing the ones that fail the net.
type Artifact struct {
	Pkg     *packages.Package
	Src     string // the digen-tagged source file (for the progress log)
	File    string
	Content []byte
}

// Process extracts a single package's injector function and returns the pending
// generated file as an Artifact. It does NOT type-check or write to disk — the
// caller (App.Run) performs one batched type-check over all artifacts and then
// writes the clean ones. Packages without a dig.Build call return
// loader.ErrNoDigBuildCall.
func (p *Processor) Process(pkg *packages.Package, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) (*Artifact, error) {
	target, err := loader.FindInjectorFunctions(pkg)
	if err != nil {
		return nil, err
	}

	// 确定输出路径
	outputPath := p.cfg.OutputFile
	if len(p.cfg.Paths) != 1 || p.cfg.Paths[0] != "." {
		if len(pkg.GoFiles) == 0 {
			return nil, fmt.Errorf("package %s has no Go files", pkg.PkgPath)
		}
		dir := filepath.Dir(pkg.GoFiles[0])
		outputPath = filepath.Join(dir, "dig_gen.go")
	}
	srcFile := target.File
	target.File = outputPath

	p.logger.Debugf("generating for package %s -> %s", pkg.PkgPath, outputPath)

	nodes, importAliasMap, pkgAliasMap, pkgNameMap, err := p.buildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		return nil, fmt.Errorf("extract and build nodes: %w", err)
	}

	refCount := make(map[string]int)
	for _, node := range nodes {
		for _, arg := range node.Args {
			refCount[arg.Name]++
		}
	}

	if p.cfg.UnusedMode == model.UnusedModeError {
		if err := p.checkUnusedProviders(nodes, refCount); err != nil {
			return nil, fmt.Errorf("unused provider check: %w", err)
		}
	}

	// The output file name embedded in the //go:generate directive must stay
	// RELATIVE (dig_gen.go) even though outputPath is absolute in ./... mode:
	// an absolute path here would bake the machine's directory layout into the
	// committed generated file and break `go generate` for other checkouts.
	code, err := p.generator.GenerateCode(nodes, refCount, pkg.Name, target.FuncName, pkg.PkgPath, importAliasMap, pkgAliasMap, pkgNameMap, pkg.Fset, target.Node.Type.Params, filepath.Base(outputPath))
	if err != nil {
		return nil, fmt.Errorf("write generated code: %w", err)
	}

	return &Artifact{Pkg: pkg, Src: srcFile, File: outputPath, Content: []byte(code)}, nil
}

// BatchVerify type-checks all pending artifacts in a single packages.Load pass
// (see generator.VerifyGeneratedBatch). It returns a map keyed by PkgPath with
// the actionable error for each artifact whose generated file failed the net;
// clean artifacts are absent.
func (p *Processor) BatchVerify(artifacts []*Artifact) map[string]error {
	if len(artifacts) == 0 {
		return nil
	}
	gas := make([]generator.GeneratedArtifact, len(artifacts))
	for i, a := range artifacts {
		gas[i] = generator.GeneratedArtifact{Pkg: a.Pkg, File: a.File, Content: a.Content}
	}
	return p.generator.VerifyGeneratedBatch(gas)
}

// WriteArtifact writes a verified artifact's generated file to disk and logs the
// per-package progress line.
func (p *Processor) WriteArtifact(a *Artifact) error {
	fmt.Printf("[digen] generated: %s -> %s\n", a.Src, a.File)
	return os.WriteFile(a.File, a.Content, 0o644)
}

// extractAndBuildNodes 原 extractAndBuildNodes 逻辑
func (p *Processor) extractAndBuildNodes(pkg *packages.Package, target *model.GenTarget, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) ([]model.Node, map[string]string, map[string]string, map[string]string, error) {
	entryFunc := target.Node
	buildCall := extractor.FindBuildCall(entryFunc, pkg.TypesInfo)
	if buildCall == nil {
		pos := pkg.Fset.Position(entryFunc.Pos())
		return nil, nil, nil, nil, fmt.Errorf("at %s: no dig.Build call found in function %s", pos, entryFunc.Name.Name)
	}

	startDir := filepath.Dir(target.File)
	extr := extractor.NewExtractor(p.cfg, pkgMap, pkg.PkgPath, strategy, startDir, p.logger)
	if err := extractor.AddExternalParams(extr, target, pkg); err != nil {
		return nil, nil, nil, nil, err
	}

	for _, arg := range buildCall.Args {
		if err := extr.ExtractOptions(arg, pkg, pkg); err != nil {
			return nil, nil, nil, nil, err
		}
	}

	nodes, err := extr.BuildFinalNodes()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return nodes, extr.ImportAliasMap(), extr.PkgAliasMap(), extr.PackageNameMap(), nil
}

// buildNodes returns the extracted IR for the package, using the on-disk IR
// cache when enabled. On a cache hit it skips the (expensive) extraction /
// type-checking step entirely. Any failure along the cache path — key
// computation, load, or save — degrades gracefully to a fresh extraction so
// the cache can never make generation fail.
func (p *Processor) buildNodes(pkg *packages.Package, target *model.GenTarget, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) ([]model.Node, map[string]string, map[string]string, map[string]string, error) {
	if !p.cfg.Cache {
		return p.extractAndBuildNodes(pkg, target, pkgMap, strategy)
	}
	cacheDir := p.cfg.CacheDir
	if cacheDir == "" {
		cacheDir = ir.DefaultCacheDir()
	}

	key, kerr := p.cacheKey(pkg)
	if kerr != nil {
		p.logger.Debugf("ir cache key computation failed, falling back to extraction: %v", kerr)
		return p.extractAndBuildNodes(pkg, target, pkgMap, strategy)
	}

	if entry, hit, lerr := ir.Load(cacheDir, key); lerr != nil {
		p.logger.Debugf("ir cache load failed, falling back to extraction: %v", lerr)
	} else if hit {
		p.logger.Debugf("ir cache hit for %s", pkg.PkgPath)
		return entry.Nodes, entry.ImportAliasMap, entry.PkgAliasMap, entry.PkgNameMap, nil
	}

	nodes, importAliasMap, pkgAliasMap, pkgNameMap, err := p.extractAndBuildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	entry := &model.CachedExtraction{
		Nodes:          nodes,
		ImportAliasMap: importAliasMap,
		PkgAliasMap:    pkgAliasMap,
		PkgNameMap:     pkgNameMap,
		SchemaVer:      model.SchemaVersion,
	}
	if serr := ir.Save(cacheDir, key, entry); serr != nil {
		p.logger.Debugf("ir cache save failed (non-fatal): %v", serr)
	}
	return nodes, importAliasMap, pkgAliasMap, pkgNameMap, nil
}

// cacheKey derives a stable key for a package's extraction from:
//   - the config knobs that affect the IR (alias style, closure inlining);
//   - the Go toolchain version (covers stdlib API changes, whose sources are
//     not hashed here);
//   - a fingerprint of the package's own source files;
//   - a fingerprint of every transitively imported package's source files.
//
// Including dependency sources means a breaking change in an imported package
// invalidates this package's cache entry, so the cache can never serve stale IR
// that would generate code against an old dependency API. The trade-off is that
// every cache-enabled run (hit or miss) fingerprints all reachable dependency
// sources; that is still cheaper than re-extracting + type-checking, and the
// cache is opt-in.
//
// The per-file fingerprint is (path, size, mtime) rather than content bytes.
// Closure bodies are extracted from dependency sources, so a dependency's
// *implementation* change must invalidate the cache — a plain export-data hash
// would miss that. Hashing file content would be precise but forces a full read
// of every reachable source on every run, which dominates the cost the cache is
// meant to save on large modules. mtime+size is the same trade-off the Go build
// cache and most incremental tools make: a file whose content changed in place
// without a size or mtime change (e.g. `touch -r` copying an old timestamp) is
// the one case that can yield a stale hit — acceptable for a developer-local,
// opt-in cache, and never a correctness bug (stale IR still type-checks against
// whatever is on disk when the generated file is built).
func (p *Processor) cacheKey(pkg *packages.Package) (string, error) {
	h := sha256.New()
	io.WriteString(h, p.cfg.AliasType)
	io.WriteString(h, "|")
	io.WriteString(h, strconv.FormatBool(p.cfg.InlineClosures))
	io.WriteString(h, "|")
	io.WriteString(h, runtime.Version())
	io.WriteString(h, "|")

	// Current package's own source files.
	if err := hashPackageFiles(h, pkg); err != nil {
		return "", err
	}

	// Transitive dependencies: fingerprinting their source files (and their
	// import paths) makes a dependency API/implementation change invalidate the
	// cache entry.
	seen := map[string]bool{pkg.PkgPath: true}
	var walk func(pk *packages.Package) error
	walk = func(pk *packages.Package) error {
		for _, imp := range pk.Imports {
			if imp.PkgPath == "" || seen[imp.PkgPath] {
				continue
			}
			seen[imp.PkgPath] = true
			io.WriteString(h, "\x00dep:")
			io.WriteString(h, imp.PkgPath)
			io.WriteString(h, "\x00")
			// A dependency whose sources cannot be fingerprinted must fail closed:
			// skipping it would let the cache key omit that dependency's content
			// and serve stale IR generated against an old dependency API.
			if err := hashPackageFiles(h, imp); err != nil {
				return err
			}
			if err := walk(imp); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(pkg); err != nil {
		return "", err
	}

	io.WriteString(h, "\x00")
	io.WriteString(h, pkg.PkgPath)
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// hashPackageFiles folds the sorted source file paths and their (size, mtime)
// fingerprint into h. It deliberately does NOT read file content: on a
// cache-enabled run every reachable dependency is fingerprinted, and content
// hashing would make that step the dominant cost instead of the extraction it
// is meant to skip. Archive-only packages (e.g. stdlib) have no GoFiles and
// only contribute their import path elsewhere; their API is covered by
// runtime.Version() in the cache key, so not hashing their sources is fine.
func hashPackageFiles(h io.Writer, pkg *packages.Package) error {
	files := append([]string{}, pkg.GoFiles...)
	files = append(files, pkg.OtherFiles...)
	sort.Strings(files)
	for _, f := range files {
		io.WriteString(h, f)
		io.WriteString(h, "\x00")
		info, err := os.Stat(f)
		if err != nil {
			return err
		}
		io.WriteString(h, strconv.FormatInt(info.Size(), 10))
		io.WriteString(h, "\x00")
		io.WriteString(h, strconv.FormatInt(info.ModTime().UnixNano(), 10))
		io.WriteString(h, "\x00")
	}
	return nil
}

// checkUnusedProviders 原 checkUnusedProviders
func (p *Processor) checkUnusedProviders(nodes []model.Node, refCount map[string]int) error {
	for _, node := range nodes {
		if node.IsInvoke {
			continue
		}
		if node.HasError {
			continue
		}
		if refCount[node.Name] == 0 {
			funcDesc := node.LongName()
			posHint := ""
			if node.Position != "" {
				posHint = fmt.Sprintf(" at %s", node.Position)
			}
			return fmt.Errorf("unused provider%s: %s (returns %s)\n  💡 Fix: either add an Invoke that consumes %s, or remove this provider; use -unused=ignore to suppress",
				posHint, funcDesc, node.RetType, node.RetType)
		}
	}
	return nil
}

// ExtractNodes returns the extracted IR nodes for a package's injector function
// without writing any generated file. Used by the graph/explain subcommands.
func (p *Processor) ExtractNodes(pkg *packages.Package, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) ([]model.Node, error) {
	target, err := loader.FindInjectorFunctions(pkg)
	if err != nil {
		return nil, err
	}
	nodes, _, _, _, err := p.buildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

// CheckPackage validates a package's DI contract (extraction + unused-provider
// check) without writing any file. Used by the check subcommand.
func (p *Processor) CheckPackage(pkg *packages.Package, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) error {
	nodes, err := p.ExtractNodes(pkg, pkgMap, strategy)
	if err != nil {
		return err
	}
	refCount := make(map[string]int)
	for _, node := range nodes {
		for _, arg := range node.Args {
			refCount[arg.Name]++
		}
	}
	if p.cfg.UnusedMode == model.UnusedModeError {
		if err := p.checkUnusedProviders(nodes, refCount); err != nil {
			return fmt.Errorf("unused provider check: %w", err)
		}
	}
	return nil
}
