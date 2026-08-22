//go:build digen

package closure_capture_timehour

import (
	"context"
	"time"

	"github.com/shanjunmei/dig"
)

// InitCaptureTimeHour 是 G2 回归场景的成功形态。
//
// 闭包体内引用 time.Hour——这是 time 包的【跨包导出】具名常量。在 B1 误报修复前，
// collectFreeVarsFromBody 仅按 o.Parent() != pkgScope 判定，把 time.Hour 这类
// 跨包导出常量错判为「当前包局部常量」而误报
// "cannot capture local constant "Hour" defined in InitApp scope"。
//
// 修复后（closure.go 增加 Exported() 闸门）跨包导出符号被直接放行。
// 本例被 example/successtest 自动发现并断言「可成功生成且可编译」，
// 从而锁定 G2/B1 误报的修复，防止回归。
func InitCaptureTimeHour() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(NewConfig),
			dig.Invoke(func(cfg *Config) {
				cfg.Interval = 24 * time.Hour // time.Hour 是 time 包级导出常量，修复后不再误报
			}),
		),
	)
}
