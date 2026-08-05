// Package pkg 提供两个返回 (T, error) 的 provider 函数，
// 用于测试 digen 生成的 err 变量名遮蔽包别名 err 的场景。
package pkg

// Item1 是第一个 provider 的返回类型。
type Item1 struct {
	Name string
}

// Item2 是第二个 provider 的返回类型。
type Item2 struct {
	Name string
}

// NewItem1 返回 Item1 和 error。
func NewItem1() (*Item1, error) {
	return &Item1{Name: "item1"}, nil
}

// NewItem2 返回 Item2 和 error。
// 当 di.go 以 `import err "....pkg"` 导入本包时，
// 生成代码中 `v0, err := err.NewItem1()` 会使 err 变量遮蔽 err 包别名，
// 导致后续 `v1, err := err.NewItem2()` 编译失败。
// ShadowGuard 应将 err 变量重命名为 err2 以避免遮蔽。
func NewItem2() (*Item2, error) {
	return &Item2{Name: "item2"}, nil
}
