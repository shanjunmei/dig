//go:build digen

package setup

import (
	"github.com/shanjunmei/dig/example/cache"
	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/db"
	"github.com/shanjunmei/dig/example/role"
	"github.com/shanjunmei/dig/example/user"

	"github.com/shanjunmei/dig"
)

// Full 返回完整依赖图的共享模块。
// 适用于需要跨包模块嵌套、命名实例、泛型、闭包 Provide/Invoke 等高级特性的入口。
func Full() dig.Option {
	return dig.Module(
		// === 跨包模块（嵌套）===
		user.Module(),
		role.Module(),
		cache.Module(),
		db.Module(),

		// === 闭包 Provide：跨包类型 user.Store[string] ===
		dig.Provide(func() *user.Store[string] {
			s := user.NewStore[string]()
			s.Add("hello")
			s.Add("world")
			return s
		}),

		// === 跨包消费命名实例（primaryDB / replicaDB）===
		dig.Invoke(func(primaryDB *db.DB, userRedis *db.RedisClient) {
			primaryDB.Ping()
			userRedis.Ping()
		}),

		// === 运行时分支（闭包内 if/else，编译期不受限）===
		dig.Invoke(func(s *user.Store[string], cfg *common.Config) {
			if cfg.Port > 8000 {
				s.Add("high-port")
			} else {
				s.Add("low-port")
			}
		}),
	)
}

// Basic 返回基础依赖图的共享模块。
// 仅包含模块嵌套，不含命名实例、泛型缓存、闭包等高级特性。
func Basic() dig.Option {
	return dig.Module(
		user.Module(),
		role.Module(),
	)
}
