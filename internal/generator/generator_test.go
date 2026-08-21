package generator

import (
	"testing"

	"github.com/shanjunmei/dig/internal/model"
)

func TestBuildIIFECall(t *testing.T) {
	// Not inline or empty def -> empty string.
	if got := buildIIFECall(model.Node{ShouldInline: false, ClosureDef: "func f() int { return 1 }"}); got != "" {
		t.Fatalf("expected empty when ShouldInline=false, got %q", got)
	}
	if got := buildIIFECall(model.Node{ShouldInline: true, ClosureDef: ""}); got != "" {
		t.Fatalf("expected empty when ClosureDef empty, got %q", got)
	}

	// Strips the function name, turning a named func into an IIFE.
	in := "func dv_foo(a int, b string) *T {\n\treturn newT(a, b)\n}"
	want := "func(a int, b string) *T {\n\treturn newT(a, b)\n}"
	if got := buildIIFECall(model.Node{ShouldInline: true, ClosureDef: in}); got != want {
		t.Fatalf("IIFE conversion:\n got: %q\nwant: %q", got, want)
	}

	// Defensive: no opening paren -> returned unchanged.
	if got := buildIIFECall(model.Node{ShouldInline: true, ClosureDef: "func foo"}); got != "func foo" {
		t.Fatalf("expected def unchanged without paren, got %q", got)
	}
}

func TestBuildIdentityConversion(t *testing.T) {
	// Not an identity closure -> empty.
	if got := buildIdentityConversion(model.Node{IsIdentityClosure: false, IdentityOp: model.OpDirect, IdentityTargetType: "T"}, "x"); got != "" {
		t.Fatalf("expected empty when IsIdentityClosure=false, got %q", got)
	}

	cases := []struct {
		op   model.OpKind
		want string
	}{
		{model.OpDirect, "T(x)"},
		{model.OpAddr, "&x"},
		{model.OpDeref, "*x"},
		{model.OpConvert, "T(x)"},
		{model.OpAssert, "x.(T)"},
	}
	for _, c := range cases {
		got := buildIdentityConversion(model.Node{IsIdentityClosure: true, IdentityOp: c.op, IdentityTargetType: "T"}, "x")
		if got != c.want {
			t.Fatalf("op=%s: got %q want %q", c.op, got, c.want)
		}
	}

	// Unknown op -> empty (safe no-op).
	if got := buildIdentityConversion(model.Node{IsIdentityClosure: true, IdentityOp: "bogus", IdentityTargetType: "T"}, "x"); got != "" {
		t.Fatalf("expected empty for unknown op, got %q", got)
	}
}
