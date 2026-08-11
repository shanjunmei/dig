// 场景：跨包 Module() 内引用了同包的未导出函数
// 预期错误：func "newHidden" is private in package ... and cannot be used from package ...
package gf_private_visibility

import (
	"context"

	"github.com/shanjunmei/dig/example/gen_failures/private_visibility/pkg"

	"github.com/shanjunmei/dig"
)

// InitPrivateVisibility 通过 pkg.Module() 引入了一个包含未导出函数 newHidden 的模块。
// newHidden 在 pkg 包内可见，但生成代码在主包 gf_private_visibility 中，
// 跨包引用未导出符号会导致编译失败。digen 应在生成阶段检测并报错。
func InitPrivateVisibility() func(context.Context) error {
	return dig.Build(
		dig.Module(
			pkg.Module(),
			dig.Invoke(func(e *pkg.Exported) {
				_ = e
			}),
		),
	)
}
