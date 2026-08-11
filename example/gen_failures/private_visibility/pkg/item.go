package pkg

import (
	"github.com/shanjunmei/dig"
)

// Hidden 是一个未导出的类型。
type hidden struct {
	Name string
}

// newHidden 是一个未导出的构造函数。
func newHidden() *hidden {
	return &hidden{Name: "hidden"}
}

// Exported 是一个导出的类型，用于对照组。
type Exported struct {
	Name string
}

// NewExported 是导出的构造函数。
func NewExported() *Exported {
	return &Exported{Name: "exported"}
}

// Module 返回 dig.Option，其中引用了同包的未导出函数 newHidden。
// 当主包通过 pkg.Module() 引入此模块时，digen 需要检查 newHidden
// 对主包是否可见。由于 newHidden 未导出，生成代码无法引用它。
func Module() dig.Option {
	return dig.Module(
		dig.Provide(newHidden),
		dig.Provide(NewExported),
	)
}
