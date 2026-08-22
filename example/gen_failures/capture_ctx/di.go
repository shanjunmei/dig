//go:build digen

// 场景：闭包捕获 context.Context 类型的包级变量
// 预期错误：cannot capture context variable "globalCtx" as free variable
//
// 注意：使用非导出名称 globalCtx，以区分「跨包导出符号合法引用」（B1 修复后
// 导出符号被放行）与「context 变量禁止作为 free variable 捕获」这两条独立规则。
// 若此处用导出名 GlobalCtx，会被 Exported() 闸门直接放行而失去本 fixture 的
// 测试意图。
package gf_capture_ctx

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 包级 context.Context 变量（非导出，避免与跨包导出符号放行规则混淆）
var globalCtx = context.Background()

func InitCaptureCtx() func(context.Context) error {
	return dig.Build(
		dig.Module(
			// 闭包捕获了包级 context.Context 变量
			dig.Provide(func() context.Context {
				return globalCtx
			}),
			dig.Invoke(func(c context.Context) {}),
		),
	)
}
