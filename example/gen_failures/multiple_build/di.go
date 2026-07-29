package gf_multiple_build

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：多个函数包含 dig.Build
// 预期错误：multiple functions containing dig.Build call found
func BuildOne() func(context.Context) error {
	return dig.Build(dig.Module(dig.Supply("a")))
}

func BuildTwo() func(context.Context) error {
	return dig.Build(dig.Module(dig.Supply("b")))
}
