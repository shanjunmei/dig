package main

// Map 处理切片 []T，单参数转换
func Map[T, R any](src []T, fn func(T) R) []R {
	res := make([]R, 0, len(src))
	for _, v := range src {
		res = append(res, fn(v))
	}
	return res
}

// MapKeys 只遍历map key，对key做映射
func MapKeys[M ~map[K]V, K comparable, V any, R any](m M, fn func(K) R) []R {
	res := make([]R, 0, len(m))
	for k := range m {
		res = append(res, fn(k))
	}
	return res
}

// MapEntries 遍历完整键值对 k+v
func MapEntries[M ~map[K]V, K comparable, V any, R any](m M, fn func(K, V) R) []R {
	res := make([]R, 0, len(m))
	for k, v := range m {
		res = append(res, fn(k, v))
	}
	return res
}

// Keys 快捷提取所有key，不用写闭包
func Keys[M ~map[K]V, K comparable, V any](m M) []K {
	return MapKeys(m, func(k K) K { return k })
}

func Reduce[T, R any](src []T, init R, fn func(R, T) R) R {
	res := init
	for _, item := range src {
		res = fn(res, item)
	}
	return res
}
