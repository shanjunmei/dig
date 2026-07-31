// 场景：InitApp 有命名返回值
// 预期错误：named return value is not allowed
package gf_init_named_return

import (
	"context"

	"github.com/shanjunmei/dig"
)

func InitNamedReturn() (fn func(context.Context) error) {
	return dig.Build(
		dig.Module(
			dig.Supply("test"),
		),
	)
}