package repository

import "github.com/shanjunmei/dig"

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewRepository[int]), // 提供 Repository[int]
		dig.Invoke(func(r *Repository[int]) { r.Add(42); r.Print() }),
	)
}
