package user

import "fmt"

type Store[T any] struct {
	items []T
}

func NewStore[T any]() *Store[T] {
	return &Store[T]{items: make([]T, 0)}
}

func (s *Store[T]) Add(item T) {
	s.items = append(s.items, item)
}

func (s *Store[T]) GetAll() []T {
	return s.items
}

func ProcessStore[T any](s *Store[T], str string) error {
	fmt.Printf("ProcessStore: items count=%d s=%s\n", len(s.items), str)
	return nil
}

// RegisterStore 包级函数，演示"闭包内裸调用同包级函数"场景。
// 详见 setup/full.go 中的 BootstrapStore 闭包调用示例。
func RegisterStore(s *Store[string]) {
	fmt.Printf("RegisterStore: items count=%d\n", len(s.items))
}
