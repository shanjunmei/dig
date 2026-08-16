//go:build digen

package gf_contract_digen_symbol

import (
	"context"

	"github.com/shanjunmei/dig"
)

// 场景：用户违反 di.go 契约 —— 在 //go:build digen 文件中定义了类型与构造器，
// 却被 dig.Build 接线引用。
//
// 正常 go build（无 digen 标签）下本文件被排除，生成的 dig_gen.go 看不到
// Repo / NewRepo，会报 "undefined: Repo" / "undefined: NewRepo"。
//
// 预期错误（internal/extractor/contract.go checkContractVisibility）：
//   digen contract violation
//   ... defined inside a //go:build digen file
//
// 这是生成器在写文件之前就给出的清晰预检，而非事后由类型检查兜底并误导为
// "internal generator bug"。
type Repo struct{}

func NewRepo() *Repo {
	return &Repo{}
}

func InitContract() func(context.Context) error {
	return dig.Build(
		dig.Module(
			dig.Provide(NewRepo),
		),
	)
}
