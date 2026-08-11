// 场景：两个 Supply 为同一类型提供默认（无名）实例
// 预期错误：duplicate binding for ... (default)
package gf_duplicate_supply

import (
	"context"

	"github.com/shanjunmei/dig"
)

type Config struct{ Port int }

// InitDuplicateSupply 两个 Supply 都用复合字面量为 *Config 类型提供默认实例。
// 复合字面量不是标识符，extractSupplyName 返回空串，两者均为 default 实例。
// 预期错误：duplicate binding for *gf_duplicate_supply.Config (default)
func InitDuplicateSupply() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Supply(&Config{Port: 8080}),
			dig.Supply(&Config{Port: 9090}),
			dig.Invoke(func(cfg *Config) {
				_ = cfg
			}),
		),
	)
}
