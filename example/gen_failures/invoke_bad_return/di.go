package gf_invoke_bad_return

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Invoke 返回非 error 类型
// 预期错误：single return value must be error
func InitInvokeBadReturn() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Supply("x"),
			dig.Invoke(func(s string) string {
				return s // 返回 string 而非 error
			}),
		),
	)
}
