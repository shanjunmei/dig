package functional

import (
	"slices"
	"testing"
)

func TestMap(t *testing.T) {
	if got := Map([]int{1, 2, 3}, func(x int) int { return x * 2 }); !slices.Equal(got, []int{2, 4, 6}) {
		t.Fatalf("Map = %v, want [2 4 6]", got)
	}
}

func TestMapEntries(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	got := MapEntries(m, func(k string, v int) string { return k + string(rune('0'+v)) })
	slices.Sort(got)
	if !slices.Equal(got, []string{"a1", "b2"}) {
		t.Fatalf("MapEntries = %v, want [a1 b2]", got)
	}
}

func TestKeys(t *testing.T) {
	m := map[string]int{"b": 2, "a": 1}
	got := Keys(m)
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("Keys = %v, want [a b]", got)
	}
}

func TestReduce(t *testing.T) {
	if got := Reduce([]int{1, 2, 3, 4}, 0, func(r, x int) int { return r + x }); got != 10 {
		t.Fatalf("Reduce = %d, want 10", got)
	}
}
