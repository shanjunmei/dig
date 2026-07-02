package processor

import (
	"fmt"
	"path/filepath"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/extractor"
	"github.com/shanjunmei/dig/internal/generator"
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

// Process 处理单个包
func (p *Processor) Process(pkg *packages.Package, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) error {
	target, err := loader.FindInjectorFunctions(pkg)
	if err != nil {
		return err
	}

	// 确定输出路径
	outputPath := p.cfg.OutputFile
	if len(p.cfg.Paths) != 1 || p.cfg.Paths[0] != "." {
		if len(pkg.GoFiles) == 0 {
			return fmt.Errorf("package %s has no Go files", pkg.PkgPath)
		}
		dir := filepath.Dir(pkg.GoFiles[0])
		outputPath = filepath.Join(dir, "dig_gen.go")
	}
	srcFile := target.File
	target.File = outputPath

	p.logger.Debugf("generating for package %s -> %s", pkg.PkgPath, outputPath)

	nodes, pkgAliasMap, err := p.extractAndBuildNodes(pkg, target, pkgMap, strategy)
	if err != nil {
		return fmt.Errorf("extract and build nodes: %w", err)
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

	if err := p.generator.WriteGeneratedCode(pkg, target, nodes, refCount, pkgAliasMap, pkg.Fset); err != nil {
		return fmt.Errorf("write generated code: %w", err)
	}

	fmt.Printf("[digen] generated: %s -> %s\n", srcFile, outputPath)
	return nil
}

// extractAndBuildNodes 原 extractAndBuildNodes 逻辑
func (p *Processor) extractAndBuildNodes(pkg *packages.Package, target *model.GenTarget, pkgMap map[string]*packages.Package, strategy alias.AliasStrategy) ([]model.Node, map[string]string, error) {
	entryFunc := target.Node
	buildCall := extractor.FindBuildCall(entryFunc, pkg.TypesInfo)
	if buildCall == nil {
		return nil, nil, fmt.Errorf("no dig.Build call found")
	}

	extr := extractor.NewExtractor(pkgMap, pkg.PkgPath, strategy)
	if err := extractor.AddExternalParams(extr, target, pkg); err != nil {
		return nil, nil, err
	}

	for _, arg := range buildCall.Args {
		if err := extr.ExtractOptions(arg, pkg, pkg); err != nil {
			return nil, nil, err
		}
	}

	nodes, err := extr.BuildFinalNodes()
	if err != nil {
		return nil, nil, err
	}
	return nodes, extr.PkgAliasMap(), nil
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
			return fmt.Errorf("unused provider: %s (returns %s)", funcDesc, node.RetType)
		}
	}
	return nil
}
