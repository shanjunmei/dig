package gf_invoke_option

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：dig.Invoke 接受 Option 作为参数
// 预期错误：dig.Invoke cannot accept another Option as argument
func InitInvokeOption() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Invoke(dig.Supply("x")),
		),
	)
}
