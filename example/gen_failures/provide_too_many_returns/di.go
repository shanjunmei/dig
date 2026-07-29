package gf_provide_too_many_returns

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Provide 返回过多值
// 预期错误：too many return values
func InitProvideTooManyReturns() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(func() (string, int, error) {
				return "a", 1, nil
			}),
			dig.Invoke(func(s string) {}),
		),
	)
}
