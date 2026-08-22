//go:build digen

package gf_init_no_return

import "github.com/shanjunmei/dig"

// 场景：入口函数没有任何返回值。
// 预期错误（internal/extractor/closure.go:1188）：
//
//	function ...: must have a return value of type func(context.Context) error
func InitNoReturn() {
	dig.Build()
}
