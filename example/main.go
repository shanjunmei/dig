package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shanjunmei/dig/example/app"
	"github.com/shanjunmei/dig/example/app_basic"
	"github.com/shanjunmei/dig/example/app_debug"
	"github.com/shanjunmei/dig/example/app_edge"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
)

type modeEntry struct {
	desc    string
	startFn func(*common.Config, *logger.Logger) func(context.Context) error
}

func main() {
	modes := map[string]modeEntry{
		"full":  {desc: "完整特性演示 (跨包模块、命名实例、泛型、闭包)", startFn: app.InitApp},
		"basic": {desc: "基础特性 (模块嵌套 + Supply)", startFn: app_basic.InitAppBasic},
		"debug": {desc: "调试模式 (与 full 相同依赖图)", startFn: app_debug.InitAppDebug},
		"edge":  {desc: "边界场景 (多命名+默认实例、泛型)", startFn: app_edge.InitAppEdge},
	}

	mode := "full"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	entry, ok := modes[mode]
	if !ok {
		fmt.Printf("Unknown mode: %s\n\n", mode)
		fmt.Println("Available modes:")
		for _, m := range []string{"full", "basic", "debug", "edge"} {
			fmt.Printf("  %-14s %s\n", m, modes[m].desc)
		}
		os.Exit(1)
	}

	cfg := common.NewConfig()
	log := logger.NewLogger()

	fmt.Printf("=== Running mode: %s (%s) ===\n\n", mode, entry.desc)

	start := entry.startFn(cfg, log)
	if err := start(context.Background()); err != nil {
		fmt.Printf("\n[FAILED] %s\n", err)
		os.Exit(1)
	}

	fmt.Println("\n[OK] completed successfully")
}
