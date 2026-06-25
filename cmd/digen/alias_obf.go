package main

import (
	"fmt"
	"hash/fnv"
)

// ObfuscatedAliasStrategy 生成最短混淆别名：单字母 + 数字后缀（仅在冲突时）
type ObfuscatedAliasStrategy struct{}

func (ObfuscatedAliasStrategy) GenerateAlias(pkgPath string, existing map[string]bool) string {
	// 1. 计算哈希
	h := fnv.New64a()
	h.Write([]byte(pkgPath))
	hashVal := h.Sum64()

	// 2. 取一个字母（大小写共 52 个）
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUWXYZabcdefghijklmnopqrstuwxyz"
	l := len(alphabet)
	base := string(alphabet[hashVal%uint64(l)])

	// 3. 尝试 base，若冲突则追加数字（1, 2, 3...）
	alias := base
	for i := 1; ; i++ {
		if !existing[alias] {
			return alias
		}
		alias = fmt.Sprintf("%s%d", base, i)
	}
}
