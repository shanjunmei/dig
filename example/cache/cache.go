package cache

import "fmt"

type Cache[T any] struct {
	data map[string]T
}

func NewCache[T any]() *Cache[T] {
	return &Cache[T]{data: make(map[string]T)}
}

func (c *Cache[T]) Set(key string, val T) {
	c.data[key] = val
}

func (c *Cache[T]) Get(key string) (T, bool) {
	v, ok := c.data[key]
	return v, ok
}

func (c *Cache[T]) Print() {
	for k, v := range c.data {
		fmt.Printf("  %s: %v\n", k, v)
	}
}
