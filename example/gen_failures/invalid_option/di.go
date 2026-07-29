package gf_invalid_option

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：Build 中传入无效表达式（不是函数调用）
// 预期错误：invalid option expression
func InitInvalidOption() func(context.Context) error {
	opt := dig.Module(dig.Supply("x"))
	return dig.Build(
		dig.Module(
			opt, // 变量引用而非直接调用
		),
	)
}
