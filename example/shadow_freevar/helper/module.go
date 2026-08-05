package helper

import "github.com/shanjunmei/dig"

// Module 返回一个 dig.Option，内含捕获 db 自由变量的闭包 provider。
// 当此闭包被提取到主包（主包以 db 为别名导入 example/db）时，
// ShadowGuard 应将自由变量参数名从 db 重命名为 db_fv，避免遮蔽包别名。
func Module() dig.Option {
	return dig.Module(
		dig.Provide(func() *Result {
			return &Result{Value: db.Count}
		}),
	)
}
