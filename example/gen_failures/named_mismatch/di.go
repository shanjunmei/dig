//go:build digen

package gf_named_mismatch

import (
	"context"

	"github.com/shanjunmei/dig"
)

type Service struct{ Name string }

func newService() (primary *Service) {
	return &Service{Name: "primary"}
}

// 场景：命名提供者不匹配 —— 参数名 "secondary" 与提供者名 "primary" 不匹配
// 预期错误：no provider for type *Service with name "secondary"
func InitNamedMismatch() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(newService),
			dig.Invoke(func(secondary *Service) error {
				return nil
			}),
		),
	)
}
