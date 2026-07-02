package main

import (
	"context"
	"fmt"

	"github.com/shanjunmei/dig/example/app"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
)

func main() {
	cfg := common.NewConfig()
	log := logger.NewLogger()

	start := app.InitApp(cfg, log)
	if err := start(context.Background()); err != nil {
		fmt.Printf("App failed: %v\n", err)
	}
}
