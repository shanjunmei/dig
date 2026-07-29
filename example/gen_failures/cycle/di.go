package gf_cycle

import (
	"context"

	"github.com/shanjunmei/dig"
)

type A struct{ b *B }
type B struct{ a *A }

func newA(b *B) *A { return &A{b: b} }
func newB(a *A) *B { return &B{a: a} }

// 场景：循环依赖
// 预期错误：circular dependency detected
func InitCycle() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(newA),
			dig.Provide(newB),
			dig.Invoke(func(a *A) {}),
		),
	)
}
