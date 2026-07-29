package cache

import (
	"fmt"

	"github.com/shanjunmei/dig"
)

func Module() dig.Option {
	return dig.Module(
		// 泛型构造器
		dig.Provide(NewCache[string]),
		dig.Provide(NewCache[int]),

		// 闭包 Provide：跨包类型（消费本包的泛型 Cache）
		dig.Provide(func() *Cache[bool] {
			c := NewCache[bool]()
			c.Set("enabled", true)
			return c
		}),

		// 消费跨包类型
		dig.Invoke(func(stringCache *Cache[string], intCache *Cache[int]) {
			stringCache.Set("greeting", "hello")
			intCache.Set("count", 42)
			fmt.Println("Cache module:")
			stringCache.Print()
			intCache.Print()
		}),
	)
}
