//go:build digen

package closure_capture_string_const

import (
	"context"

	"github.com/shanjunmei/dig"
)

// fontName 是包级字符串常量。
//
// 闭包体内直接引用它时，collectFreeVarsFromBody（internal/extractor/closure.go）
// 走 *types.Const 分支将其交给 extractConstLiteral 转成字面量后内联，生成的代码
// 里常量引用被替换为字符串字面量。修复前 extractConstLiteral 对 constant.String
// 用 strconv.Quote(val.String()) 二次转义，而 constant.Value.String() 对字符串
// 常量已返回带引号形式（"font:cjk"），二次包裹后内联出 "\"font:cjk\""，运行期
// 拿到的是带字面引号的字符串而非 font:cjk。
const fontName = "font:cjk"

// InitCaptureStringConst 是字符串常量二次转义 bug（font:cjk）的回归场景。
//
// dig.Invoke 闭包内引用【同包包级】字符串常量 fontName，触发上面的内联路径。
// 修复后生成代码中 cfg.Font 应被赋值为 "font:cjk"（无多余引号）。
//
// 本例被 example/successtest 自动发现并断言「可成功生成且可编译」，
// 并由 example/golden 锁定生成产物逐字节一致，防止回归。
func InitCaptureStringConst() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(NewConfig),
			dig.Invoke(func(cfg *Config) {
				cfg.Font = fontName
			}),
		),
	)
}
