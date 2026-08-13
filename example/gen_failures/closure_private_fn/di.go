// 场景：跨包 Module() 内的闭包体中调用了同包的未导出函数
// 预期错误：func "buildAuditAuthorizer" is private in package ... and cannot be used from package ...
package gf_closure_private_fn

import (
	"context"

	"github.com/shanjunmei/dig"
	"github.com/shanjunmei/dig/example/gen_failures/closure_private_fn/pkg"
)

// InitClosurePrivateFn 通过 pkg.Module() 引入了一个闭包，该闭包内部
// 调用了 pkg 包中未导出的 buildAuditAuthorizer。当 digen 把闭包提升
// 到主包时，会生成 pkg.buildAuditAuthorizer 这种非法引用。
func InitClosurePrivateFn() func(context.Context) error {
	return dig.Build(
		dig.Module(
			pkg.Module(),
		),
	)
}
