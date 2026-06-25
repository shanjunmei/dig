package main

import (
	"fmt"
	"strings"
)

func replacePkgChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '-' {
			return '_'
		}
		return r
	}, s)
}

// AliasStrategy 定义别名生成策略
type AliasStrategy interface {
	GenerateAlias(pkgPath string, existing map[string]bool) string
}

// SimpleAliasStrategy 简化版：最后一段 + 数字
type SimpleAliasStrategy struct{}

func (SimpleAliasStrategy) GenerateAlias(pkgPath string, existing map[string]bool) string {
	parts := strings.Split(pkgPath, "/")
	base := replacePkgChars(parts[len(parts)-1])
	alias := base
	for i := 2; existing[alias]; i++ {
		alias = fmt.Sprintf("%s%d", base, i)
	}
	return alias
}

// ContextualAliasStrategy 复杂版：逐级拼接
type ContextualAliasStrategy struct{}

func (ContextualAliasStrategy) GenerateAlias(pkgPath string, existing map[string]bool) string {
	parts := strings.Split(pkgPath, "/")
	if len(parts) == 0 {
		return "pkg"
	}
	for i := 1; i <= len(parts); i++ {
		alias := strings.Join(parts[len(parts)-i:], "_")
		alias = replacePkgChars(alias)
		if len(alias) > 0 && alias[0] >= '0' && alias[0] <= '9' {
			alias = "_" + alias
		}
		if !existing[alias] {
			return alias
		}
	}
	fullAlias := strings.ReplaceAll(pkgPath, "/", "_")
	fullAlias = replacePkgChars(fullAlias)
	if !existing[fullAlias] {
		return fullAlias
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", fullAlias, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

// NewAliasStrategy 工厂函数，根据参数返回对应策略
func NewAliasStrategy(useSimple bool) AliasStrategy {
	if useSimple {
		return SimpleAliasStrategy{}
	}
	return ContextualAliasStrategy{}
}
