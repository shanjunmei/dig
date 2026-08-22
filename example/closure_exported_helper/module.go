package closure_exported_helper

import (
	"github.com/shanjunmei/dig"
)

// ExportedVar 是一个包级导出变量。digen 的 free-var 捕获逻辑
// （internal/extractor/closure.go 的 collectFreeVarsFromBody）此前缺少
// Exported() 闸门，会把跨包引用导出符号误报为
// "cannot capture local variable ... defined in InitApp scope"。
// 本包的导出符号用于锁定该误报的修复：合法跨包导出引用必须被放行。
var ExportedVar = "exported-value"

// ExportedConst 是一个包级导出常量，复现同一误报的另一分支
// （"cannot capture local constant ..."）。
const ExportedConst = 42

// Config 是构造器的返回类型，供生成目标包消费。
type Config struct {
	Label string
	Limit int
}

// NewConfig 依据本包的导出符号构造 Config。闭包体里直接引用
// closure_exported_helper.ExportedVar / .ExportedConst，正是 B1 误报的
// 触发形态。
func NewConfig() Config {
	return Config{
		Label: ExportedVar,
		Limit: ExportedConst,
	}
}

// Module 提供 NewConfig，供生成目标包 example/closure_capture_exported 消费。
func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewConfig),
	)
}
