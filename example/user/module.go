package user

import (
	"github.com/shanjunmei/dig"
	"github.com/shanjunmei/dig/example/user/repository"
)

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewStore[int]),
		repository.Module(),
		dig.Provide(func() string { return "user-module" }),
		dig.Invoke(ProcessStore[int]),
	)
}
