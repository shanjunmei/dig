package gf_multi_module

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：多个 dig.Module 调用
// 预期错误：function InitMultiModule contains multiple dig.Module calls
func InitMultiModule() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Supply("a"),
		),
		dig.Module(
			dig.Supply("b"),
		),
	)
}
