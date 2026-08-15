//go:build digen

package closure_param

import (
	"context"

	"github.com/shanjunmei/dig/example/closure_param_helper"

	"github.com/shanjunmei/dig"
)

// InitClosureParam 是 closure_param_helper.Module() 的生成目标包。
//
// 因为 Module() 内的闭包定义于 closure_param_helper 包，而本包是生成目标包
// （mainPkgPath = example/closure_param），闭包参数 f 的 Pkg() 与 mainPkgPath
// 不同——这正是此前误报 "var \"f\" is private ... (generation target)" 的触发条件。
//
// 该例被 example/successtest 自动发现并断言「可成功生成且可编译」，从而锁定
// internal/extractor/visibility.go 中 isPackageLevelVar 守卫的修复，防止回归。
func InitClosureParam() func(context.Context) error {
	return dig.Build(
		closure_param_helper.Module(),
	)
}
