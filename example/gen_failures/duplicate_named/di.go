// 场景：两个 provider 为同一类型提供相同的命名实例
// 预期错误：duplicate binding for ... with name "primary"
package gf_duplicate_named

import (
	"context"

	"github.com/shanjunmei/dig"
)

type Service struct{ Name string }

func newPrimary1() (primary *Service) {
	return &Service{Name: "primary-1"}
}

func newPrimary2() (primary *Service) {
	return &Service{Name: "primary-2"}
}

// InitDuplicateNamed 两个 provider 都返回命名实例 "primary"（同类型 *Service）。
// 预期错误：duplicate binding for *Service with name "primary"
func InitDuplicateNamed() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(newPrimary1),
			dig.Provide(newPrimary2),
			dig.Invoke(func(primary *Service) {
				_ = primary
			}),
		),
	)
}
