package supply_param_helper

import (
	"github.com/shanjunmei/dig"
)

// Config 故意导出，聚焦"supply 的值是定义包的函数参数"这一误报，
// 与"类型本身跨包不可见"无关。
type Config string

// Module 复现误报场景：dig.Supply(cfg) 的 cfg 是本函数（Module）的参数，
// 不是包级符号。当该 Module 被内联进生成目标包时，cfg 应作为自由变量被
// 引用（如 dv0 := cfg），而非被错误限定为 supply_param_helper.cfg
// （未导出，目标包无法引用 → undefined: supply_param_helper.cfg）。
func Module(cfg *Config) dig.Option {
	return dig.Module(
		dig.Supply(cfg),
		dig.Invoke(func(c *Config) {
			_ = c
		}),
	)
}
