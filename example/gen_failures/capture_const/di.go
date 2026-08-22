//go:build digen

// 场景：闭包捕获局部变量
// 预期错误：cannot capture local variable "maxRetries" defined in InitApp scope
//
// 注意：使用函数内局部变量（非包级、非导出）以触发 local-variable 捕获拦截。
// 原用例用局部常量 const MaxRetries，但 digen 对局部常量的处理是内联其字面量
// 而非在生成期拦截，反而会落到 type-check 失败（internal generator bug），
// 无法稳定断言「捕获拦截」语义。变量形态能稳定触发 local-variable 拦截分支。
package gf_capture_const

import (
	"context"

	"github.com/shanjunmei/dig"
)

func InitCaptureConst() func(context.Context) error {
	var maxRetries = 3
	return dig.Build(
		dig.Module(
			// 闭包捕获了局部变量 maxRetries
			dig.Provide(func() int {
				return maxRetries
			}),
			dig.Invoke(func(n int) {}),
		),
	)
}
