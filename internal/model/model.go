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

type GenTarget struct {
	FuncName string
	Node     *ast.FuncDecl
	File     string
}

type Node struct {
	Name      string
	Func      string
	FuncPkg   string
	RetType   string
	Args      []string
	IsInvoke  bool
	IsSupply  bool
	Value     string
	HasError  bool
	IsClosure bool

	ClosureDef string
	UsedPkgs   []string
	PkgPath    string

	IsConstArg     []bool
	ConstLitValues []string
	IsContextArg   []bool
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
