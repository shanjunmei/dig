//go:build digen

package gf_no_module

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：入口函数含 dig.Build，且用 dig.Module 包裹了一个命名辅助函数；
// 但该辅助函数体内只写了 dig.Provide，却忘记用 dig.Module 包裹。
// 提取器在 extractOptionsFromFuncCall 中校验辅助函数体必须包含 dig.Module，
// 否则在 internal/extractor/extractor.go:291 报错。
// 预期错误（internal/extractor/extractor.go:291）：
//
//	function ... does not contain dig.Module
func helperWithoutModule() dig.Option {
	return dig.Provide(func() string { return "x" })
}

func InitNoModule() func(context.Context) error {
	return dig.Build(
		dig.Module(helperWithoutModule()),
	)
}
