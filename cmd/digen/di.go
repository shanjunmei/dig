//go:build digen

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen

package main

import (
	"context"
	"log"

	"github.com/shanjunmei/dig/internal/app"
	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/generator"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/processor"

	"github.com/shanjunmei/dig"
	"github.com/shanjunmei/dig/pkg/alias"
)

// InitApp 使用 dig 组装所有组件，返回应用入口函数
func InitApp(cfg *config.Config) func(context.Context) error {
	return dig.Build(
		// 基础组件
		dig.Provide(logger.NewLogger),
		dig.Provide(loader.NewPackageLoader),
		dig.Provide(func(_cfg *config.Config) string { return _cfg.AliasType }),
		dig.Provide(func(_aliasType string) alias.AliasStrategy {
			aliasType, err := alias.ParseAliasType(_aliasType)
			if err != nil {
				log.Fatalln(err)
			}
			return alias.NewAliasStrategy(aliasType)
		}),

		// 核心组件
		dig.Provide(app.NewApp),
		dig.Provide(generator.NewGenerator),
		dig.Provide(processor.NewProcessor),

		// 启动
		dig.Invoke(func(a *app.App) error {
			return a.Run()
		}),
	)
}
