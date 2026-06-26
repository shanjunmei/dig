package alias

import "fmt"

// NumericAliasStrategy 生成数字混淆别名：_1, _2, _3 ...
// 注意：此策略不保证确定性，别名顺序取决于生成时的解析顺序。
type NumericAliasStrategy struct {
	counter int
}

func (s *NumericAliasStrategy) GenerateAlias(pkgPath string, existing map[string]bool) string {
	// 从当前计数器值开始尝试，如果被占用则递增
	for {
		s.counter++
		alias := fmt.Sprintf("_%d", s.counter)
		if !existing[alias] {
			return alias
		}
		// 如果被占用，继续递增
	}
}
