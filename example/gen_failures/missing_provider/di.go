//go:build digen

package gf_missing_provider

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Invoke 需要一个没有 Provider 的类型
// 预期错误：no provider for type *struct{} with name "missing"
func InitMissingProvider() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Supply("hello"),
			dig.Invoke(func(s string, missing *struct{}) error {
				return nil
			}),
		),
	)
}
