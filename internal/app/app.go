package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/processor"

	"github.com/shanjunmei/dig/pkg/alias"
)

type App struct {
	processor     *processor.Processor
	loader        *loader.PackageLoader
	logger        *logger.Logger
	aliasStrategy alias.AliasStrategy
	cfg           *config.Config
}

func NewApp(processor *processor.Processor, loader *loader.PackageLoader, logger *logger.Logger, aliasStrategy alias.AliasStrategy, cfg *config.Config) *App {
	return &App{
		processor:     processor,
		loader:        loader,
		logger:        logger,
		aliasStrategy: aliasStrategy,
		cfg:           cfg,
	}
}

func (a *App) Run() error {
	start := time.Now()

	a.logger.Debugf("alias strategy: %s", a.cfg.AliasType)

	pkgs, pkgMap, err := a.loader.Load(a.cfg.Paths)
	if err != nil {
		return err
	}

	// Phase 1: extract + generate every package into in-memory artifacts. No
	// file is written yet.
	var failedCount int
	var failedErrors []string
	var artifacts []*processor.Artifact
	for _, pkg := range pkgs {
		art, err := a.processor.Process(pkg, pkgMap, a.aliasStrategy)
		if err != nil {
			if errors.Is(err, loader.ErrNoDigBuildCall) {
				continue
			}
			a.logger.Debugf("failed to process package %s: %v", pkg.PkgPath, err)
			failedErrors = append(failedErrors, fmt.Sprintf("  Package %s:\n    %s", pkg.PkgPath, err.Error()))
			failedCount++
			continue
		}
		artifacts = append(artifacts, art)
	}

	// Phase 2: one batched type-check over all generated files (when the net is
	// enabled), then write only the artifacts that pass. This turns the
	// per-package re-load of the whole dependency graph into a single pass —
	// on `digen ./...` the net is O(packages) loads instead of one per package.
	generatedCount := 0
	var verifyErrs map[string]error
	if a.cfg.TypeCheckNet && len(artifacts) > 0 {
		verifyErrs = a.processor.BatchVerify(artifacts)
	}
	for _, art := range artifacts {
		if err := verifyErrs[art.Pkg.PkgPath]; err != nil {
			a.logger.Debugf("type-check net rejected package %s: %v", art.Pkg.PkgPath, err)
			failedErrors = append(failedErrors, fmt.Sprintf("  Package %s:\n    %s", art.Pkg.PkgPath, err.Error()))
			failedCount++
			continue
		}
		if err := a.processor.WriteArtifact(art); err != nil {
			failedErrors = append(failedErrors, fmt.Sprintf("  Package %s:\n    %s", art.Pkg.PkgPath, err))
			failedCount++
			continue
		}
		generatedCount++
	}

	if generatedCount == 0 && failedCount == 0 {
		return fmt.Errorf("no packages with dig.Build found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error")
	}

	// The summary line is ALWAYS emitted last. On failure it is appended to the
	// returned error (which main prints to stderr), so under a long scroll the
	// tail of the output still shows the overall result — a user who only
	// catches the end must not misread a partial failure as a total failure.
	summary := fmt.Sprintf("[digen] generated %d/%d packages (%d failed), cost: %s",
		generatedCount, generatedCount+failedCount, failedCount, time.Since(start))
	if failedCount > 0 {
		// Partial failure must surface a non-zero exit so CI pipelines fail
		// instead of silently going green while some packages were not generated.
		return fmt.Errorf("%d package(s) failed to generate (%d succeeded):\n%s\n%s",
			failedCount, generatedCount, strings.Join(failedErrors, "\n"), summary)
	}
	fmt.Println(summary)
	return nil
}
