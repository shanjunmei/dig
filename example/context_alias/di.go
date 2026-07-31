//go:build digen

// 场景：用户为 context 包使用自定义别名
// 验证生成代码正确使用别名
package context_alias

import (
	ctx "context"

	"github.com/shanjunmei/dig"
)

func InitWithContextAlias() func(ctx.Context) error {
	return dig.Build(
		dig.Module(
			dig.Invoke(func(c ctx.Context) {}),
		),
	)
}
