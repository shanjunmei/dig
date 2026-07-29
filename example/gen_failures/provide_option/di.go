package gf_provide_option

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：dig.Provide 接受 Option 作为参数
// 预期错误：dig.Provide cannot accept another Option as argument
func InitProvideOption() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(dig.Supply("x")),
		),
	)
}
