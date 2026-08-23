package generator

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestBuildIssueReportURL verifies the pre-filled GitHub "new issue" URL carries
// the title and body as query parameters, so a user can click once to report.
func TestBuildIssueReportURL(t *testing.T) {
	const title = "digen generated code failed type-check (internal generator bug)"
	const body = "## What happened\nline1\nline2"
	got := buildIssueReportURL(title, body)

	if !strings.HasPrefix(got, digenIssueBaseURL+"?") {
		t.Fatalf("unexpected URL prefix: %q", got)
	}
	q, err := url.ParseQuery(strings.TrimPrefix(got, digenIssueBaseURL+"?"))
	if err != nil {
		t.Fatalf("URL query not parseable: %v (url=%q)", err, got)
	}
	if q.Get("title") != title {
		t.Fatalf("title param mismatch: got %q want %q", q.Get("title"), title)
	}
	if q.Get("body") != body {
		t.Fatalf("body param mismatch: got %q want %q", q.Get("body"), body)
	}
}

// TestBuildGeneratorBugReport verifies the bug-report body embeds the generated
// file path and the raw error locations, so a pasted template is self-contained.
func TestBuildGeneratorBugReport(t *testing.T) {
	const genFile = "/abs/dig_gen.go"
	const locs = "  dig_gen.go:12:5: undefined: Foo\n"
	got := buildGeneratorBugReport(genFile, locs)

	for _, want := range []string{"## What happened", locs, genFile, "## Environment", "## How to reproduce"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bug report missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// TestUndefinedMainPkgSymbol verifies the classifier extracts the undefined
// symbol name (stripping any method receiver qualifier) and rejects non-undef
// messages.
func TestUndefinedMainPkgSymbol(t *testing.T) {
	cases := []struct {
		msg  string
		want string
		ok   bool
	}{
		{"undefined: Foo", "Foo", true},
		{"undefined: Foo.Bar", "Foo", true}, // receiver qualifier stripped
		{"undefined: foo", "foo", true},
		{"cannot use int as string", "", false},
		{"some other error", "", false},
	}
	for _, c := range cases {
		name, ok := undefinedMainPkgSymbol(c.msg)
		if ok != c.ok || name != c.want {
			t.Fatalf("undefinedMainPkgSymbol(%q) = (%q,%v), want (%q,%v)", c.msg, name, ok, c.want, c.ok)
		}
	}
}

// TestSplitErrorPos verifies robust extraction of the file portion of an error
// position, tolerant of ':' inside Windows drive-letter paths.
func TestSplitErrorPos(t *testing.T) {
	cases := []struct {
		pos  string
		file string
		ok   bool
	}{
		{"a/b.go:12:5", "a/b.go", true},
		{"c:\\path\\dig_gen.go:12:5", "c:\\path\\dig_gen.go", true},
		{"onlyfile.go:42", "onlyfile.go", true},
		{"", "", false},
		{"nofile", "", false},
	}
	for _, c := range cases {
		file, ok := splitErrorPos(c.pos)
		if ok != c.ok || file != c.file {
			t.Fatalf("splitErrorPos(%q) = (%q,%v), want (%q,%v)", c.pos, file, ok, c.file, c.ok)
		}
	}
}

// TestErrorInGeneratedFile verifies only errors whose position refers to the
// generated file are attributed to the net (so stale on-disk errors are ignored).
func TestErrorInGeneratedFile(t *testing.T) {
	const gen = "/abs/dig_gen.go"
	if !errorInGeneratedFile(packages.Error{Pos: gen + ":12:5", Msg: "undefined: X"}, gen) {
		t.Fatal("expected error in generated file to be attributed")
	}
	if errorInGeneratedFile(packages.Error{Pos: "other.go:1:1", Msg: "undefined: Y"}, gen) {
		t.Fatal("expected error in other file to be ignored")
	}
	if errorInGeneratedFile(packages.Error{Pos: ""}, gen) {
		t.Fatal("empty position must not match")
	}
}
