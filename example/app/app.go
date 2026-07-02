//go:build digen

package app

import (
	"context"

	"github.com/shanjunmei/dig/example/common"
	"github.com/shanjunmei/dig/example/internal/logger"
	"github.com/shanjunmei/dig/example/role"

	//	roleRepo "github.com/shanjunmei/dig/example/role/repository" // 同名模块
	"github.com/shanjunmei/dig/example/user"

	"github.com/shanjunmei/dig"
)

func InitApp(cfg *common.Config, log *logger.Logger) func(context.Context) error {
	return dig.Build(
		// 外部参数注入
		//	dig.Supply(cfg),
		//dig.Provide(func() *logger.Logger { return log }),

		// 用户模块（内部已包含 user/repository.Module()）
		user.Module(),

		// 角色模块（内部已包含 role/repository.Module()）
		role.Module(),

		//	roleRepo.Module(),

		// 额外闭包 Provide
		dig.Provide(func() *user.Store[string] {
			s := user.NewStore[string]()
			s.Add("hello")
			return s
		}),

		// 闭包 Invoke
		dig.Invoke(func(s *user.Store[string], cfg *common.Config, log *logger.Logger) error {
			log.Println("App Invoke: store len =", len(s.GetAll()))
			return nil
		}),
	)
}
