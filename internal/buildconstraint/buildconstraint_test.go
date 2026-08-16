package buildconstraint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestRequiresDigen(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"digen", true},
		{" digen ", true},
		{"digen && foo", true},
		{"foo || digen", true},
		{"(digen || linux) && amd64", true},
		{"digen && !dev", true},
		{"foo && bar", false},
		{"linux && amd64", false},
		{"!digen", false}, // requires the ABSENCE of digen
		{"", false},
		{"digen_extra", false}, // substring must be a standalone token
	}
	for _, c := range cases {
		if got := RequiresDigen(c.expr); got != c.want {
			t.Errorf("RequiresDigen(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestFileHasDigenConstraint(t *testing.T) {
	parse := func(src string) *ast.File {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "di_gen.go", src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return f
	}

	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"go:build digen before package", "//go:build digen\n\npackage main\n", true},
		{"+build digen before package", "// +build digen\n\npackage main\n", true},
		{"go:build !digen", "//go:build !digen\n\npackage main\n", false},
		{"no constraint", "package main\n", false},
		{"constraint after package clause", "package main\n\n//go:build digen\n", false},
		{"unrelated constraint", "//go:build linux && amd64\n\npackage main\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parse(tt.src)
			if got := FileHasDigenConstraint(f); got != tt.want {
				t.Errorf("FileHasDigenConstraint = %v, want %v", got, tt.want)
			}
		})
	}
}
