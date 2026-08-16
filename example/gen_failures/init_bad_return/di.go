//go:build digen

package gf_init_bad_return

import "github.com/shanjunmei/dig"

// 场景：入口函数返回类型不是 func(context.Context) error（此处返回 int）。
// 预期错误（internal/extractor/closure.go:1202）：
//   function ...: invalid return type "int", expected func(context.Context) error
func InitBadReturn() int {
	dig.Build()
	return 0
}
