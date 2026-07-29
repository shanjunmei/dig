package gf_provide_bad_error

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Provide 第二返回值不是 error
// 预期错误：second return value must be error
func InitProvideBadError() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(func() (string, string) {
				return "a", "b" // 第二返回值不是 error
			}),
			dig.Invoke(func(s string) {}),
		),
	)
}
