//go:build digen

// 场景：闭包内声明与外层变量同名时，外层的非法引用不得被漏放行。
//
// 内层 `x := 2` 让名字 x 出现在闭包体内。若按名字判定「是否定义在闭包内」
// （旧的 declSet 方案），第一个 `y := x` 引用的外层 x 会被误认为闭包内定义
// 而跳过，digen 会生成 dig_provider_1() { y := x } —— 生成成功但产物里
// undefined: x。
//
// 正确行为：按对象声明位置判定，外层 x 的声明不在闭包字面量区间内，必须报错。
package closure_shadow_outer

import (
	"context"

	"github.com/shanjunmei/dig"
)

func Init(x int) func(context.Context) error {
	return dig.Build(
		dig.Provide(func() *Repo {
			y := x
			x := 2
			_ = x
			return &Repo{N: y}
		}),
		dig.Invoke(func(r *Repo) error {
			_ = r
			return nil
		}),
	)
}
