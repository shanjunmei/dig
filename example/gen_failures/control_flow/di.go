package gf_control_flow

import (
	"context"

	"github.com/shanjunmei/dig"
)

// helper 函数将 dig.Module 放在控制流内
// 预期错误：function helper contains dig.Module inside control flow
func helper() dig.Option {
	if true {
		return dig.Module(
			dig.Supply("init"),
		)
	}
	return nil
}

func InitControlFlow() func(context.Context) error {
	return dig.Build(
		helper(),
	)
}
