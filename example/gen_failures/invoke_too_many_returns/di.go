package gf_invoke_too_many_returns

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Invoke 返回多个值
// 预期错误：has 2 return values (only 0 or error allowed)
func InitInvokeTooManyReturns() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Supply("x"),
			dig.Invoke(func(s string) (error, string) {
				return nil, s // 返回两个值
			}),
		),
	)
}
