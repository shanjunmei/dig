// Package buildconstraint provides helpers for inspecting Go build
// constraints that are relevant to digen.
//
// A source file is considered "digen-scoped" when its //go:build (or legacy
// +build) constraint requires the "digen" tag. This notion is needed in two
// independent places — the extractor (to validate the di.go contract) and the
// generator's type-check safety net (to classify "undefined" errors) — so it
// lives here as a single leaf package with no internal dependencies, keeping
// both callers free of duplicated logic and import cycles.
package buildconstraint

import (
	"go/ast"
	"strings"
)

// RequiresDigen reports whether a build-constraint expression (the text after
// //go:build / +build, i.e. with the leading "go:build"/"+build" already
// stripped) requires the "digen" tag.
//
// It tolerates the common boolean operators (&&, ||) and grouping parentheses
// by splitting the expression into whitespace/operator-delimited tokens and
// looking for the literal token "digen". A bare "digen", "digen && foo", or
// "foo || digen" all return true; "foo && bar" returns false.
func RequiresDigen(expr string) bool {
	expr = strings.ReplaceAll(expr, "&&", " ")
	expr = strings.ReplaceAll(expr, "||", " ")
	tokens := strings.FieldsFunc(expr, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '(' || r == ')' || r == '&' || r == '|'
	})
	for _, t := range tokens {
		if t == "digen" {
			return true
		}
	}
	return false
}

// FileHasDigenConstraint reports whether f carries a //go:build (or +build)
// constraint that requires the "digen" tag.
//
// Build constraints must appear before the package clause, so any comment
// located after the package keyword is ignored.
func FileHasDigenConstraint(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.End() > f.Package {
			continue
		}
		for _, c := range cg.List {
			body := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			switch {
			case strings.HasPrefix(body, "go:build"):
				if RequiresDigen(strings.TrimSpace(body[len("go:build"):])) {
					return true
				}
			case strings.HasPrefix(body, "+build"):
				if RequiresDigen(strings.TrimSpace(body[len("+build"):])) {
					return true
				}
			}
		}
	}
	return false
}
