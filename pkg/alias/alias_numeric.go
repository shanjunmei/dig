package alias

import (
	"fmt"
	"hash/fnv"
)

// NumericAliasStrategy 生成数字混淆别名：_1, _2, ...
//
// 旧实现依赖一个全局计数器，别名完全取决于生成时的解析顺序：只要包的遍历
// 顺序变化（并行、map 迭代、加载顺序差异），同一组包就得到不同的别名，破坏
// 了「同输入→同产物」的确定性。
//
// 本实现改为：别名起点由包路径的 FNV-1a 哈希决定。同一包路径始终得到同一
// 起点，因此别名分配与解析/遍历顺序无关（确定性）。仅当不同包路径哈希冲突
// （起点相同）且该槽位已被占用时，才从固定起点顺序向后探测下一个空闲数字，
// 此时结果仍然确定（探测推进顺序固定）。
type NumericAliasStrategy struct{}

func (s *NumericAliasStrategy) GenerateAlias(pkgPath string, existing map[string]bool) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(pkgPath))
	start := int(h.Sum32()%10000) + 1 // 1..10000
	n := start
	for {
		alias := fmt.Sprintf("_%d", n)
		if !existing[alias] {
			return alias
		}
		n++
		if n > 10000 {
			n = 1
		}
		if n == start {
			// 所有槽位都被占用（数千个包路径的极端情况），退化为确定性后缀，
			// 保证不产生冲突且仍可复现。
			return fmt.Sprintf("_%d_%x", start, h.Sum32())
		}
	}
}
