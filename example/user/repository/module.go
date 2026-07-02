package repository

import "github.com/shanjunmei/dig"

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewRepository[string]), // 提供 Repository[string]
		dig.Invoke(func(r *Repository[string]) { r.Print() }),
	)
}
