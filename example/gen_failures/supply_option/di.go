package gf_supply_option

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：dig.Supply 接受 Option 作为参数
// 预期错误：dig.Supply cannot accept another Option as argument
func InitSupplyOption() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Supply(dig.Supply("x")),
		),
	)
}
