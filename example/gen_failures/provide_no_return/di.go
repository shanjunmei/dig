package gf_provide_no_return

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Provide 函数无返回值
// 预期错误：func has no return
func InitProvideNoReturn() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(func() {
				// 没有返回值
			}),
			dig.Invoke(func() {}),
		),
	)
}
