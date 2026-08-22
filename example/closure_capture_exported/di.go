//go:build digen

package closure_capture_exported

import (
	"context"

	"github.com/shanjunmei/dig/example/closure_exported_helper"

	"github.com/shanjunmei/dig"
)

// InitCaptureExported 是 closure_exported_helper.Module() 的生成目标包。
//
// 该闭包体直接引用跨包导出的变量与常量
// （closure_exported_helper.ExportedVar / .ExportedConst）。在 B1 误报修复前，
// collectFreeVarsFromBody 对 *types.Var / *types.Const 仅按
// o.Parent() != pkgScope 判定，会把这种合法的跨包导出引用误报为
// "cannot capture local variable/constant ... defined in InitApp scope"。
//
// B1 修复（closure.go 增加 Exported() 闸门）放行了这类引用，但暴露了生成侧的
// 另一处缺陷：collectTypeNameAndUsedPkgs 此前只处理 *types.TypeName 与
// *types.Func，漏掉了 *types.Var / *types.Const。闭包被提升到主包后，裸写的
// ExportedVar / ExportedConst 会编译失败（undefined: ExportedVar），这正是
// hermes 的 "undefined: TransportStdio" 根因（mcp.TransportStdio 是 mcp 包导出的
// 常量，被 dig.Invoke 闭包引用）。
//
// 真正的修复（方案1）在 collectTypeNameAndUsedPkgs 增加 *types.Var / *types.Const
// 分支：当符号来自非主包时，将其裸标识符改写为 <alias>.<Name>。本例被
// example/successtest 自动发现并断言「可成功生成且可编译」，从而锁定该修复。
func InitCaptureExported() func(context.Context) error {
	return dig.Build(
		closure_exported_helper.Module(),
		dig.Invoke(func() error {
			// 直接引用跨包导出的变量与常量；生成代码必须改写为
			// closure_exported_helper.ExportedVar / .ExportedConst。
			_ = closure_exported_helper.ExportedVar
			_ = closure_exported_helper.ExportedConst
			return nil
		}),
	)
}
