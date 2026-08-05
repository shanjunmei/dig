//go:build digen

// 场景：自由变量参数名遮蔽包别名
// 验证 digen 提取闭包时，自由变量参数名不与主包的包别名冲突
//
// 根因：helper 包有包级变量 db（*helper.Counter），闭包捕获它作为自由变量。
// 主包以 db 为别名导入 example/db（alias=full 策略下最后一段为 db）。
// 提取闭包时，若参数名仍为 db，会遮蔽主包的 db 包别名。
// 修复：ShadowGuard 检测到 db 是保留名，将参数重命名为 db_fv。
package shadow_freevar

import (
	"context"

	"github.com/shanjunmei/dig/example/db"
	"github.com/shanjunmei/dig/example/shadow_freevar/helper"

	"github.com/shanjunmei/dig"
)

// InitShadowFreeVar 主包导入 example/db（别名为 db），
// 同时使用 helper.Module() 提供的闭包（捕获名为 db 的自由变量）。
func InitShadowFreeVar() func(context.Context) error {
	return dig.Build(
		helper.Module(),
		dig.Supply(&db.DB{Name: "test"}),
		dig.Invoke(func(r *helper.Result, Db *db.DB) {
			_ = r
			_ = Db
		}),
	)
}
