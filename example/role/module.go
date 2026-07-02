package role

import (
	"github.com/shanjunmei/dig/example/role/repository"

	"github.com/shanjunmei/dig"
)

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewServer),
		dig.Supply(100),
		repository.Module(), // 引用子模块（同名但路径不同）
		dig.Invoke(func(s *Server) { s.Run() }),
	)
}
