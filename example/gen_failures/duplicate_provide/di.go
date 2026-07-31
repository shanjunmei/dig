// 场景：闭包内 Provide 同一类型两次
// 预期错误：duplicate provide for type
package gf_duplicate_provide

import (
	"context"

	"github.com/shanjunmei/dig"
)

func InitDuplicateProvide() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(func() string { return "a" }),
			dig.Provide(func() string { return "b" }),
			dig.Invoke(func(s string) {}),
		),
	)
}