// 场景：闭包捕获局部常量
// 预期错误：cannot capture local constant "MaxRetries" defined in InitApp scope
package gf_capture_const

import (
	"context"

	"github.com/shanjunmei/dig"
)

func InitCaptureConst() func(context.Context) error {
	const MaxRetries = 3
	return dig.Build(
		dig.Module(
			// 闭包捕获了局部常量 MaxRetries
			dig.Provide(func() int {
				return MaxRetries
			}),
			dig.Invoke(func(n int) {}),
		),
	)
}