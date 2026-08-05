//go:build digen

// 场景：包别名与 err 变量名冲突
// 验证 digen 生成的 err 变量名不会遮蔽用户显式指定的包别名 err
//
// 根因：writeProvider 中 `vN, err := pkg.Func()` 的 err 变量声明在外层作用域，
// 后续 provider 调用 `err.NewItem2()` 中的 err 会解析为变量而非包别名，编译失败。
// 修复：ShadowGuard 检测到 err 是保留名（包别名），自动改用 err2。
package shadow_err

import (
	"context"

	err "github.com/shanjunmei/dig/example/shadow_err/pkg"

	"github.com/shanjunmei/dig"
)

// InitShadowErr 两个 provider 均返回 (T, error)，且来自以 err 为别名的包。
// 若 err 变量名未被 ShadowGuard 重命名，生成的代码将无法编译。
func InitShadowErr() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(err.NewItem1),
			dig.Provide(err.NewItem2),
			dig.Invoke(func(i1 *err.Item1, i2 *err.Item2) {
				_ = i1
				_ = i2
			}),
		),
	)
}
