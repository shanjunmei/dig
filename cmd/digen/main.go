package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/model"
)

func main() {
	// 解析命令行参数
	outputFile := flag.String("out", "dig_gen.go", "output file name")
	unusedModeStr := flag.String("unused", "error", "behavior for unused providers: error, ignore, drop")
	debug := flag.Bool("debug", false, "enable debug logging")
	aliasStr := flag.String("alias", "full", "alias generation style: short, full, obfuscated, numeric")
	showVersion := flag.Bool("version", false, "print version information and exit")
	inlineClosures := flag.Bool("inline", false, "inline simple closures as IIFE (Phase 3)")
	typeCheckNet := flag.Bool("typecheck", true, "type-check generated code to catch internal generator bugs (disable for large ./... runs)")
	cache := flag.Bool("cache", false, "cache the extracted IR to disk and reuse it for unchanged packages (skips extraction on cache hit)")
	cacheDir := flag.String("cachedir", "", "IR cache directory (default: os.TempDir()/digen-ir-cache); ignored unless -cache is set")
	flag.Parse()

	if *showVersion {
		fmt.Println(versionString())
		return
	}

	cfg := &config.Config{
		OutputFile: *outputFile,
		UnusedMode: parseUnusedMode(unusedModeStr),
		Debug:      *debug,

		AliasType:      *aliasStr,
		Paths:          flag.Args(),
		InlineClosures: *inlineClosures,
		TypeCheckNet:   *typeCheckNet,
		Cache:          *cache,
		CacheDir:       *cacheDir,
	}
	if len(cfg.Paths) == 0 {
		cfg.Paths = []string{"."}
	}

	// 使用 dig 构建应用并运行
	run := InitApp(cfg)
	if err := run(context.Background()); err != nil {
		log.Fatalf("application error: %v", err)
	}
}

func parseUnusedMode(s *string) model.UnusedMode {
	switch *s {
	case "ignore":
		return model.UnusedModeIgnore
	case "drop":
		return model.UnusedModeDrop
	default:
		return model.UnusedModeError
	}
}
