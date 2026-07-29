package gf_duplicate_param

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：InitApp 有重复类型的参数
// 预期错误：duplicate parameter type
func InitDuplicateParam(a string, b string) func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Invoke(func(a string, b string) {}),
		),
	)
}
