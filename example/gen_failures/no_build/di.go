package gf_no_build

import "github.com/shanjunmei/dig"

// 场景：没有 dig.Build 调用
// 预期错误：no function containing dig.Build call found
func NoBuild() dig.Option {
	// 只有 dig.Module，没有 dig.Build
	return dig.Module()
}
