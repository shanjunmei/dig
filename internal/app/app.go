package app

import (
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

	// 创建别名策略

	a.logger.Debugf("alias strategy: %s", a.cfg.AliasType)

	pkgs, pkgMap, err := a.loader.Load(a.cfg.Paths)
	if err != nil {
		return err
	}

	var generatedCount, failedCount int
	for _, pkg := range pkgs {
		if err := a.processor.Process(pkg, pkgMap, a.aliasStrategy); err != nil {
			if strings.Contains(err.Error(), "no function containing dig.Build call found") {
				continue
			}
			failedCount++
		} else {
			generatedCount++
		}
	}

	if generatedCount == 0 {
		return fmt.Errorf("no packages with dig.Build found")
	}
	fmt.Printf("total %d packages, %d generated, %d failed, cost: %s",
		generatedCount+failedCount, generatedCount, failedCount, time.Since(start))
	return nil
}
