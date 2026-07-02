package role

import (
	"fmt"

	"github.com/shanjunmei/dig/example/role/repository"

	"github.com/shanjunmei/dig"
)

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewServer),
		dig.Supply(100),
		repository.Module(),
		dig.Supply(Config("production")),
		//dig.Supply("hello"),
		// 新增 Invoke 消费 Config，使其被使用
		dig.Invoke(func(cfg Config) {
			fmt.Printf("Config supplied: %s\n", cfg)
		}),
		dig.Invoke(func(s *Server) { s.Run() }),
	)
}
