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

		// === 身份闭包内联场景集合（4 种 OpKind 全覆盖） ===

		// OpDirect（接口包装）：返回值即参数（隐式转换为接口）
		// 消费 primaryDB（*db.DB），生产 db.Pinger（接口）
		dig.Provide(func(primaryDB *db.DB) db.Pinger {
			return primaryDB
		}),

		// OpConvert（显式接口包装）：显式 any(...) 调用
		// 消费 primaryDB（*db.DB），生产 any（空接口）
		dig.Provide(func(primaryDB *db.DB) any {
			return any(primaryDB)
		}),

		// OpAddr（取地址）：从值类型取地址得到指针
		// 先构造一个值类型，再用 &v 形式提供 *Config（命名实例 setupCfg，避免与 InitApp 参数冲突）
		dig.Provide(func() common.Config {
			return common.Config{Addr: "127.0.0.1", Port: 9000}
		}),
		dig.Provide(func(cfg common.Config) (setupCfg *common.Config) {
			return &cfg
		}),

		// OpDeref（解引用）：从指针解引用得到值类型
		// 先构造 *DB 值（命名实例 closureDB），再用 *p 形式提供 DB（值拷贝）
		dig.Provide(func() (closureDB *db.DB) {
			return &db.DB{Name: "inmem", DSN: "mem://"}
		}),
		dig.Provide(func(closureDB *db.DB) db.DB {
			return *closureDB
		}),

		// === 跨包消费命名实例（primaryDB / replicaDB）===
		dig.Invoke(func(primaryDB *db.DB, userRedis *db.RedisClient) {
			primaryDB.Ping()
			userRedis.Ping()
		}),

		// === 消费身份闭包产物（验证内联正确性） ===
		dig.Invoke(func(pinger db.Pinger, raw any, setupCfg *common.Config, dbVal db.DB, closureDB *db.DB) {
			pinger.Ping()
			if d, ok := raw.(*db.DB); ok {
				d.Ping()
			}
			_ = setupCfg.Port
			_ = dbVal.Name
			closureDB.Ping()
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
