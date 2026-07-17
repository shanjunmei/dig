package user

import (
	"github.com/shanjunmei/dig"
	"github.com/shanjunmei/dig/example/user/repository"
)

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewStore[int]),
		repository.Module(),
		dig.Provide(func() (str string, err error) { str = "user-module"; return }),
		dig.Invoke(ProcessStore[int]),
	)
}
