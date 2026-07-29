//go:build digen

package app_debug

import (
	"context"

	"github.com/shanjunmei/dig/example/cache"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
	"github.com/shanjunmei/dig/example/setup"
	"github.com/shanjunmei/dig/example/user"

	"github.com/shanjunmei/dig"
)

// InitAppDebug 与 InitApp 共用 setup.Full() 依赖图，
// 仅入口级差异：Supply 值和 Invoke 日志信息不同。
// 使用 -debug=true 生成时，运行时注入 Logf 日志。
func InitAppDebug(cfg *common.Config, log *logger.Logger) func(context.Context) error {
	return dig.Build(
		setup.Full(),

		dig.Supply("app-debug"),

		dig.Invoke(func(s *user.Store[string], cfg *common.Config, log *logger.Logger) error {
			log.Println("Debug App Invoke: store len =", len(s.GetAll()))
			return nil
		}),

		dig.Invoke(func(stringCache *cache.Cache[string]) {
			stringCache.Set("from-debug", "cross-package")
			stringCache.Print()
		}),
	)
}
