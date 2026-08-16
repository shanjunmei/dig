//go:build digen

package gf_init_multi_return

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：入口函数返回两个值（多返回值）。
// 预期错误（internal/extractor/closure.go:1191）：
//   function ...: only a single return value allowed, expected func(context.Context) error
func InitMultiReturn() (func(context.Context) error, error) {
	dig.Build()
	return nil, nil
}
