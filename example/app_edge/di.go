//go:build digen

package app_edge

import (
	"context"

	"github.com/shanjunmei/dig/example/cache"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/db"
	"github.com/shanjunmei/dig/example/internal/logger"
	"github.com/shanjunmei/dig/example/setup"

	"github.com/shanjunmei/dig"
)

// InitAppEdge 边界场景（全部成功）：
//
//  1. 多命名实例 + 默认实例 — 同类型 *db.DB 同时存在 default 和 named
//  2. 跨包泛型消费 — 消费 setup 共享模块中的泛型 cache.Cache[T]
//  3. 日志注入 — 通过 dig 注入 logger，而非闭包捕获
//  4. 多模块嵌套 — 在 setup.Full() 基础上叠加额外的 Provider 和 Invoke
func InitAppEdge(cfg *common.Config, log *logger.Logger) func(context.Context) error {
	return dig.Build(
		setup.Full(),

		// === 场景 1: 多命名实例 + 默认实例（同一类型 *db.DB 既有 named 又有 default）===
		// db.Module() 已提供 named 实例 (primaryDB, replicaDB)
		// 这里额外提供一个 default 实例用于对比
		dig.Provide(func() *db.DB {
			return &db.DB{Name: "default", DSN: "postgres://default:5432/app"}
		}),

		// === 场景 2: 消费默认 vs 命名实例 ===
		dig.Invoke(func(defaultDB *db.DB, primaryDB *db.DB, log *logger.Logger) {
			log.Println("Edge: default DB =", defaultDB.Name, ", primary DB =", primaryDB.Name)
		}),

		// === 场景 3: 泛型缓存 ===
		dig.Invoke(func(stringCache *cache.Cache[string], log *logger.Logger) {
			stringCache.Set("edge", "boundary-test")
			stringCache.Print()
		}),

		// === 场景 4: 多模块嵌套 ===
		dig.Invoke(func(cfg *common.Config, log *logger.Logger) error {
			log.Println("Edge: port =", cfg.Port)
			return nil
		}),
	)
}
