//go:build digen

// 场景：同类型有多个命名提供者，消费方未指定参数名
// 预期错误：no provider for type *Service (available: primary, secondary)
package gf_ambiguous

import (
	"context"

	"github.com/shanjunmei/dig"
)

type Service struct{ Name string }

func newPrimary() (primary *Service) {
	return &Service{Name: "primary"}
}

func newSecondary() (secondary *Service) {
	return &Service{Name: "secondary"}
}

// InitAmbiguous 提供两个命名实例 (primary, secondary)，
// 但 Invoke 参数名为 s（不匹配任何命名），且无默认提供者。
// 预期错误：no provider for type *Service ... (available: primary, secondary)
func InitAmbiguous() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(newPrimary),
			dig.Provide(newSecondary),
			dig.Invoke(func(s *Service) {
				_ = s
			}),
		),
	)
}
