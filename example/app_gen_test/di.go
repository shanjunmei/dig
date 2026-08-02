//go:build digen

package app_gen_test

import (
	"context"

	"github.com/shanjunmei/dig/example/common"

	"github.com/shanjunmei/dig"
)

// InitGenTest 测试函数类型作为依赖时的跨包引用收集。
// 复现场景:provider 返回值 / invoke 参数为函数类型,函数签名内引用了 common 包。
func InitGenTest() func(context.Context) error {
	return dig.Build(
		// 场景1: provider(命名函数)返回函数类型,返回类型是 *types.Signature
		dig.Provide(makeConfigHandler),

		// 场景2: invoke 闭包参数为函数类型,参数类型是 *types.Signature
		dig.Invoke(func(handler func(*common.Config) error) error {
			return handler(&common.Config{Addr: "test", Port: 8080})
		}),
	)
}
