package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shanjunmei/dig/example/app"
	"github.com/shanjunmei/dig/example/app_basic"
	"github.com/shanjunmei/dig/example/app_debug"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
)

func main() {
	mode := "full"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	cfg := common.NewConfig()
	log := logger.NewLogger()

	var start func(context.Context) error

	switch mode {
	case "basic":
		start = app_basic.InitAppBasic(cfg, log)
	case "debug":
		start = app_debug.InitAppDebug(cfg, log)
	case "full":
		start = app.InitApp(cfg, log)
	default:
		fmt.Printf("Unknown mode: %s\n", mode)
		fmt.Println("Available modes: full, basic, debug")
		os.Exit(1)
	}

	fmt.Printf("=== Running mode: %s ===\n\n", mode)

	if err := start(context.Background()); err != nil {
		fmt.Printf("App failed: %v\n", err)
	}
}
