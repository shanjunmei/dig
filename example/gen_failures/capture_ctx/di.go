// 场景：闭包捕获 context.Context 类型的包级变量
// 预期错误：cannot capture context variable "GlobalCtx" as free variable
package gf_capture_ctx

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 包级 context.Context 变量
var GlobalCtx = context.Background()

func InitCaptureCtx() func(context.Context) error {
	return dig.Build(
		dig.Module(
			// 闭包捕获了包级 context.Context 变量
			dig.Provide(func() context.Context {
				return GlobalCtx
			}),
			dig.Invoke(func(c context.Context) {}),
		),
	)
}
