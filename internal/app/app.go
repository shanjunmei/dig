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

	var generatedCount, failedCount int
	var failedErrors []string
	for _, pkg := range pkgs {
		if err := a.processor.Process(pkg, pkgMap, a.aliasStrategy); err != nil {
			if strings.Contains(err.Error(), "no function containing dig.Build call found") {
				continue
			}
			a.logger.Debugf("failed to process package %s: %v", pkg.PkgPath, err)
			failedErrors = append(failedErrors, fmt.Sprintf("  Package %s:\n    %s", pkg.PkgPath, err.Error()))
			failedCount++
		} else {
			generatedCount++
		}
	}

	if generatedCount == 0 {
		if failedCount > 0 {
			msg := fmt.Sprintf("%d package(s) with dig.Build found but failed to generate:\n%s", failedCount, strings.Join(failedErrors, "\n"))
			return errors.New(msg)
		}
		return fmt.Errorf("no packages with dig.Build found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error")
	}
	if failedCount > 0 {
		fmt.Printf("[digen] failed packages:\n%s\n", strings.Join(failedErrors, "\n"))
	}
	fmt.Printf("[digen] generated %d/%d packages (%d failed), cost: %s\n", generatedCount, generatedCount+failedCount, failedCount, time.Since(start))
	return nil
}
