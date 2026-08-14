// 场景：provider（构造函数）声明了 context.Context 参数
// 预期错误：provider "..." declares a context.Context parameter "ctx",
//           but providers are resolved eagerly inside InitApp before the runtime
//           context.Context is available
//
// 说明：provider 在 InitApp 内被急切解析，早于运行时 context.Context（即返回给
// func(context.Context) error 的那个 ctx）的产生，因此 provider 永远拿不到 ctx。
// 若不加拦截，digen 会在 InitApp 外层作用域写出 `NewDB(ctx)`，而 ctx 只在内层
// 函数定义，导致生成的代码无法编译（go build 报 undefined: ctx）。
// 正确做法：去掉构造函数的 context.Context 参数，改用 dig.Supply(...) 传入所需值，
// 或把依赖 ctx 的逻辑放到 dig.Invoke(func(ctx context.Context) {...}) 中。
package gf_provider_ctx

import (
	"context"

	"github.com/shanjunmei/dig"
)

// DB 示例依赖类型。
type DB struct{}

// NewDB 错误地在构造函数参数中使用了 context.Context。
// digen 应在生成期拦截：provider 不能声明 context.Context 参数。
func NewDB(ctx context.Context) *DB {
	_ = ctx
	return &DB{}
}

// InitProviderCtx 触发 provider-context 拦截。
func InitProviderCtx() func(context.Context) error {
	return dig.Build(
		dig.Provide(NewDB),
		dig.Invoke(func(db *DB) {}),
	)
}
