package db

import (
	"fmt"

	"github.com/shanjunmei/dig"
)

func Module() dig.Option {
	return dig.Module(
		// 命名实例注入：每个命名实例需要独立的 dig.Provide
		// 返回值参数名 = 实例名
		dig.Provide(func() (primaryDB *DB, err error) {
			primaryDB = &DB{Name: "primary", DSN: "postgres://primary:5432/app"}
			return
		}),

		dig.Provide(func() (replicaDB *DB, err error) {
			replicaDB = &DB{Name: "replica", DSN: "postgres://replica:5432/app"}
			return
		}),

		// Redis 命名实例
		dig.Provide(func() (userRedis *RedisClient) {
			userRedis = &RedisClient{Name: "user", Address: "redis://user:6379"}
			return
		}),

		dig.Provide(func() (sessionRedis *RedisClient) {
			sessionRedis = &RedisClient{Name: "session", Address: "redis://session:6379"}
			return
		}),

		// 消费命名实例（指定参数名）
		dig.Invoke(func(primaryDB *DB, userRedis *RedisClient) {
			primaryDB.Ping()
			userRedis.Ping()
		}),

		dig.Invoke(func(replicaDB *DB, sessionRedis *RedisClient) {
			fmt.Printf("Replica: %s, Session: %s\n", replicaDB.Name, sessionRedis.Name)
		}),
	)
}
