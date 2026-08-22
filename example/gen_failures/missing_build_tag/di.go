package gf_missing_build_tag

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：源文件含 dig.Build(...) 但缺少 //go:build digen 约束。
// 预期错误（internal/extractor/buildtag.go:39）：
//
//	file contains dig.Build(...) but is missing the `//go:build digen` build constraint
//
// 说明：不带该标签时，普通 go build 会同时编译本文件与生成的 dig_gen.go
// （后者带 //go:build !digen），导致 "InitApp redeclared"。这是生成器守住的
// "铁的不变量"，必须有专门夹具覆盖（此前无对应 gen_failures 用例）。
//
// 注意：本文件刻意不带 //go:build digen，因此普通 go build 会正常编译它，
// 但 digen 在生成期必须在此处报错。
func InitMissingTag() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(func() string { return "x" }),
			dig.Invoke(func(s string) {}),
		),
	)
}
