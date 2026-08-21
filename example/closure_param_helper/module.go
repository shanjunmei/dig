package closure_param_helper

import (
	"fmt"

	"github.com/shanjunmei/dig"
)

// Config 故意导出：本回归场景要聚焦的是「闭包参数名未导出」导致的误报，
// 与「类型本身跨包不可见」无关。若 Config 也未导出，则生成目标包根本无法
// 引用它，那是真实的跨包可见性问题，会把测试意图搅混，故此处导出。
type Config string

// Animal / Dog 用于锁定类型断言恒等闭包（OpAssert）的端到端生成。
// 关键回归点：断言类型 Dog 与返回类型 Animal 不同，生成器必须输出 d.(Dog)
// 而非 d.(Animal)，否则语义改变（原闭包断言到具体类型，生成的若断言到接口
// 则会接受任意实现者而与原 panic 行为不符）。
type Animal interface{ Speak() string }

type Dog struct{}

func (Dog) Speak() string { return "woof" }

// NewDog 以 any 形式提供 Dog，作为类型断言闭包 d.(Dog) 的注入来源。
func NewDog() any { return Dog{} }

// NewConfigFactory 提供 func() Config，作为闭包参数 f 的注入来源。
func NewConfigFactory() func() Config {
	return func() Config { return "closure-param-value" }
}

// Module 复现原始误报场景：闭包参数使用未导出名称 f，且 f 本身被当作函数调用。
//
// 该闭包定义在 closure_param_helper 包，但会被 digen 内联进生成目标包
// example/closure_param（二者不同包）。这正是此前误报
//
//	var "f" is private in package ...closure_param_helper
//	and cannot be used from package ...closure_param (generation target)
//
// 的触发条件：闭包参数 f 是一个 *types.Var，其 Pkg() 为 closure_param_helper，
// 不等于生成目标包 mainPkgPath。修复见 internal/extractor/visibility.go 的
// isPackageLevelVar 守卫——只有包级 var 才可能被跨包引用，闭包参数必须直接放行。
//
// 本 Module 被 example/closure_param 的生成目标包消费，并由 example/successtest
// 自动发现、断言「可成功生成且可编译」，从而防止该误报回归。
func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewConfigFactory),
		dig.Provide(NewDog),
		// 类型断言恒等闭包：any -> Animal，但真实塌缩应为 d.(Dog)。
		// 该例同时被 golden 测试钉住，确保生成器输出 d.(Dog) 而非 d.(Animal)。
		// 注意：用 Invoke 消费 Animal，避免 unused 丢弃后留下悬空死表达式。
		dig.Provide(func(d any) Animal { return d.(Dog) }),
		dig.Invoke(func(f func() Config) {
			fmt.Println("invoked closure param:", f())
		}),
		dig.Invoke(func(a Animal) {
			_ = a.Speak()
		}),
	)
}
