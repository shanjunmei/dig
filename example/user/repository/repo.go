package repository

import "fmt"

type Repository[T any] struct {
	data []T
}

func NewRepository[T any]() *Repository[T] {
	return &Repository[T]{data: make([]T, 0)}
}

func (r *Repository[T]) Add(item T) {
	r.data = append(r.data, item)
}

func (r *Repository[T]) Print() {
	fmt.Printf("UserRepo: %v\n", r.data)
}
