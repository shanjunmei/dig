package main

import (
	"context"
	"flag"
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
	flag.Parse()

	cfg := &config.Config{
		OutputFile: *outputFile,
		UnusedMode: parseUnusedMode(unusedModeStr),
		Debug:      *debug,
		AliasType:  *aliasStr,
		Paths:      flag.Args(),
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
