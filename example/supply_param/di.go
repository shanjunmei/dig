//go:build digen

package supply_param

import (
	"context"

	"github.com/shanjunmei/dig/example/supply_param_helper"

	"github.com/shanjunmei/dig"
)

// InitSupplyParam 是 supply_param_helper.Module() 的生成目标包。
// Module 内 dig.Supply(cfg) 的 cfg 是 helper 包函数参数，应作为自由变量
// 引用，而非被限定为 supply_param_helper.cfg。被 example/successtest 自动
// 发现并可编译，从而锁定本修复。
func InitSupplyParam(cfg *supply_param_helper.Config) func(context.Context) error {
	return dig.Build(
		supply_param_helper.Module(cfg),
	)
}
