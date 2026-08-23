package app

import (
	"testing"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/generator"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/processor"

	"github.com/shanjunmei/dig/pkg/alias"
)

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
