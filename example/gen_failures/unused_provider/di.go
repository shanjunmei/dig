// 场景：存在未被任何 Invoke 或其他 provider 消费的 provider（默认 -unused=error）
// 预期错误：unused provider: ... (returns ...)
package gf_unused_provider

import (
	"context"

	"github.com/shanjunmei/dig"
)

type Used struct{}
type Unused struct{}

func newUsed() *Used     { return &Used{} }
func newUnused() *Unused { return &Unused{} }

// InitUnusedProvider 注册了 newUnused 但没有任何 Invoke 或其他 provider 消费 *Unused。
// 默认 -unused=error 模式下，digen 会报错。
// 预期错误：unused provider: ... (returns *gf_unused_provider.Unused)
func InitUnusedProvider() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(newUsed),
			dig.Provide(newUnused),
			dig.Invoke(func(u *Used) {
				_ = u
			}),
		),
	)
}
