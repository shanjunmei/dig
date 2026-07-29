//go:build digen

package app_basic

import (
	"context"

	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
	"github.com/shanjunmei/dig/example/setup"

	"github.com/shanjunmei/dig"
)

// InitAppBasic 演示基础特性：仅模块嵌套 + Supply。
// 不含命名实例、泛型缓存、闭包等高级特性。
func InitAppBasic(cfg *common.Config, log *logger.Logger) func(context.Context) error {
	return dig.Build(
		setup.Basic(),

		dig.Invoke(func(str string, cfg *common.Config, log *logger.Logger) error {
			log.Println("Basic:", str, "on port", cfg.Port)
			return nil
		}),
	)
}
