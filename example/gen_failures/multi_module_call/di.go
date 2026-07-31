// 场景：辅助函数中包含多个 dig.Module 调用
// 预期错误：contains multiple dig.Module calls
package gf_multi_module_call

import (
	"context"

	"github.com/shanjunmei/dig"
)

func multiModule() dig.Option {
	m1 := dig.Module(
		dig.Supply("a"),
	)
	m2 := dig.Module(
		dig.Supply("b"),
	)
	_ = m1
	_ = m2
	return dig.Module()
}

func InitMultiModuleCall() func(context.Context) error {
	return dig.Build(
		multiModule(),
		dig.Invoke(func(s string) {}),
	)
}