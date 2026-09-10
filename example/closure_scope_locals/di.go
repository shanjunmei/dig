//go:build digen

// 场景：闭包体内（含任意嵌套作用域）定义的变量不得被当成自由变量。
//
// 「是否定义在闭包内」必须按对象声明位置判定（obj.Pos() 是否落在闭包字面量
// 区间内），而不是按标识符名字匹配。旧的 declSet 名字白名单无法枚举
// range 的 Key/Value 与嵌套 FuncLit 的参数，会把它们误报为
// "cannot capture local variable"，导致完全合法的代码无法生成。
//
// 本例覆盖：:= 声明、range Key/Value、嵌套 FuncLit 的参数与其内部的 := 声明、
// type switch guard、if-init 变量、命名返回值。
// 被 example/successtest 自动发现并断言「可成功生成且可编译」，
// 并由 example/golden 锁定生成产物逐字节一致。
package closure_scope_locals

import (
	"context"

	"github.com/shanjunmei/dig"
)

func InitScopeLocals() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(func() *Repo {
				xs := []int{1, 2, 3}
				s := 0
				for i, v := range xs {
					s += v * i
				}
				f := func(x int) int { return x * 2 }
				s += f(s)
				g := func() int {
					inner := 10
					return inner
				}
				s += g()
				var anyv any = s
				switch t := anyv.(type) {
				case int:
					s += t
				}
				if half := s / 2; half > 0 {
					s += half
				}
				return &Repo{N: s}
			}),
			dig.Provide(func() (r *Named) {
				r = &Named{V: 7}
				return
			}),
			dig.Invoke(func(repo *Repo, r *Named) error {
				_, _ = repo, r
				return nil
			}),
		),
	)
}
