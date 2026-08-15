package alias

import "testing"

func TestSimpleAliasStrategy(t *testing.T) {
	s := SimpleAliasStrategy{}
	if got := s.GenerateAlias("github.com/foo/bar", map[string]bool{}); got != "bar" {
		t.Fatalf("base alias = %q, want %q", got, "bar")
	}
	// collision handling appends a numeric suffix
	if got := s.GenerateAlias("github.com/foo/bar", map[string]bool{"bar": true}); got != "bar2" {
		t.Fatalf("collision alias = %q, want %q", got, "bar2")
	}
	// result must never collide with the existing set
	if got := s.GenerateAlias("github.com/foo/bar", map[string]bool{"bar": true, "bar2": true}); got == "bar" || got == "bar2" {
		t.Fatalf("alias %q collides with existing set", got)
	}
}

func TestContextualAliasStrategy(t *testing.T) {
	s := ContextualAliasStrategy{}
	// last path segment when free
	if got := s.GenerateAlias("github.com/foo/bar/baz", map[string]bool{}); got != "baz" {
		t.Fatalf("base alias = %q, want %q", got, "baz")
	}
	// climbs one level on collision
	if got := s.GenerateAlias("github.com/foo/bar/baz", map[string]bool{"baz": true}); got != "bar_baz" {
		t.Fatalf("collision alias = %q, want %q", got, "bar_baz")
	}
	if got := s.GenerateAlias("github.com/foo/bar/baz", map[string]bool{"baz": true, "bar_baz": true}); got == "baz" || got == "bar_baz" {
		t.Fatalf("alias %q collides with existing set", got)
	}
}

func TestObfuscatedAliasStrategy_Deterministic(t *testing.T) {
	s := ObfuscatedAliasStrategy{}
	a := s.GenerateAlias("github.com/foo/bar", map[string]bool{})
	b := s.GenerateAlias("github.com/foo/bar", map[string]bool{})
	if a != b {
		t.Fatalf("obfuscated strategy not deterministic: %q vs %q", a, b)
	}
	if len(a) != 1 {
		t.Fatalf("expected single-letter base, got %q", a)
	}
	// collision appends digits, never returns an existing alias
	existing := map[string]bool{a: true}
	got := s.GenerateAlias("github.com/foo/bar", existing)
	if existing[got] {
		t.Fatalf("alias %q already in existing set", got)
	}
	if got != a+"1" {
		t.Fatalf("expected %q on first collision, got %q", a+"1", got)
	}
}

func TestNumericAliasStrategy(t *testing.T) {
	s := &NumericAliasStrategy{}
	first := s.GenerateAlias("github.com/foo/a", map[string]bool{})
	if first != "_1" {
		t.Fatalf("first numeric alias = %q, want %q", first, "_1")
	}
	second := s.GenerateAlias("github.com/foo/b", map[string]bool{})
	if second != "_2" {
		t.Fatalf("second numeric alias = %q, want %q", second, "_2")
	}
	// respects an already-occupied slot
	s2 := &NumericAliasStrategy{}
	got := s2.GenerateAlias("github.com/foo/c", map[string]bool{"_1": true})
	if got != "_2" {
		t.Fatalf("expected to skip occupied _1, got %q", got)
	}
}
