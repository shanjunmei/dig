package model

import (
	"go/ast"
)

type UnusedMode int

const (
	UnusedModeError UnusedMode = iota
	UnusedModeIgnore
	UnusedModeDrop
)

func (m UnusedMode) String() string {
	switch m {
	case UnusedModeError:
		return "error"
	case UnusedModeIgnore:
		return "ignore"
	case UnusedModeDrop:
		return "drop"
	default:
		panic("Unknown Unused Mode")
	}
}

type GenTarget struct {
	FuncName string
	Node     *ast.FuncDecl
	File     string
}
type Arg struct {
	Name       string // 参数变量名
	IsConst    bool   // 是否常量
	ConstValue string // 常量字面值（若 IsConst 为 true）
	IsContext  bool   // 是否 context.Context
}
type Node struct {
	Name      string
	Func      string
	FuncPkg   string
	RetType   string
	Args      []Arg
	IsInvoke  bool
	IsSupply  bool
	Value     string
	HasError  bool
	IsClosure bool

	ClosureDef string
	UsedPkgs   []string
	PkgPath    string

	GenericArgs string

	Comment string
}

// fullFuncName 返回 包别名.函数名
func FullFuncName(pkgAlias, funcName string) string {
	if pkgAlias == "" {
		return funcName
	}
	return pkgAlias + "." + funcName
}

// ShortName 返回用于调用的简短名称（包别名.函数名）
func (node Node) ShortName() string {
	return FullFuncName(node.FuncPkg, node.Func)
}

// LongName 返回用于日志的完整路径（包路径.函数名）
func (node Node) LongName() string {
	if node.PkgPath == "" {
		return node.Func
	}
	return node.PkgPath + "." + node.Func
}
