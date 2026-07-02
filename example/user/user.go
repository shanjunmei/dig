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

func ProcessStore[T any](s *Store[T]) error {
	fmt.Printf("ProcessStore: items count=%d\n", len(s.items))
	return nil
}
