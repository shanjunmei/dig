//go:build digen

package app_xpkg_generic

import (
	"context"
	"fmt"

	"github.com/shanjunmei/dig/example/cache"
	"github.com/shanjunmei/dig/example/common"

	"github.com/shanjunmei/dig"
)

// InitXPkgGeneric 演示跨包泛型类型参数场景：
//
//  1. 泛型类型的类型实参本身来自另一个包：cache.Cache[*common.Config]
//     — cache 包定义 Cache[T]，T 实例化为 *common.Config（跨包指针类型）
//     — 验证 collectUsedPkgsFromType 能正确收集类型实参中的跨包引用
//  2. 命名函数 Provide 泛型构造器，类型实参为跨包指针类型
//  3. 闭包 Invoke 消费跨包泛型类型，闭包体内调用跨包泛型方法
func InitXPkgGeneric() func(context.Context) error {
	return dig.Build(
		dig.Module(
			// 场景1: 命名函数 Provide 泛型构造器，类型实参为跨包指针类型
			// cache.NewCache[*common.Config]() → 生成代码需同时导入 cache 和 common
			dig.Provide(cache.NewCache[*common.Config]),

			// 场景2: Supply 跨包类型值，作为缓存内容
			dig.Supply(&common.Config{Addr: "0.0.0.0", Port: 8080}),

			// 场景3: 闭包 Invoke 消费跨包泛型类型，闭包体内调用跨包泛型方法
			// cfgCache 参数类型为 *cache.Cache[*common.Config]，涉及两个跨包
			dig.Invoke(func(cfgCache *cache.Cache[*common.Config], cfg *common.Config) error {
				cfgCache.Set("default", cfg)
				cached, ok := cfgCache.Get("default")
				if !ok {
					return fmt.Errorf("config not found in cache")
				}
				fmt.Printf("XGen: cached config addr=%s port=%d\n", cached.Addr, cached.Port)
				return nil
			}),
		),
	)
}
