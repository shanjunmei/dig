//go:build digen

package app

import (
	"context"

	"github.com/shanjunmei/dig/example/cache"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
	"github.com/shanjunmei/dig/example/setup"
	"github.com/shanjunmei/dig/example/user"

	"github.com/shanjunmei/dig"
)

// InitApp 演示 dig 全部核心特性：
//
//  1. 跨包模块嵌套 — user.Module() 内部嵌套 user/repository.Module()
//  2. 同包名别名   — role/repository 与 user/repository 同包名，digen 自动生成别名
//  3. 命名实例注入 — db.Module() 返回 primaryDB/replicaDB 两个同名类型不同实例
//  4. 泛型支持     — cache.Cache[T]、user.Store[T]、repository.Repository[T]
//  5. 闭包 Provide — 内联构造闭包（仅允许包级变量和字面量）
//  6. 闭包 Invoke  — 启动时执行闭包，消费跨包类型
//  7. Supply       — 直接注入任意值
//  8. 外部参数     — InitApp 顶层入参自动注入
func InitApp(cfg *common.Config, log *logger.Logger) func(context.Context) error {
	return dig.Build(
		setup.Full(),

		dig.Supply("app-v2"),

		dig.Invoke(func(s *user.Store[string], cfg *common.Config, log *logger.Logger) error {
			log.Println("App Invoke: store len =", len(s.GetAll()))
			return nil
		}),

		dig.Invoke(func(stringCache *cache.Cache[string]) {
			stringCache.Set("from-app", "cross-package")
			stringCache.Print()
		}),
	)
}
