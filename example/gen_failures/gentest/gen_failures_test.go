// Package gentest provides an automated regression test for the
// example/gen_failures/ fixtures.
//
// Each subdirectory of example/gen_failures/ is a "source-as-sample"
// fixture whose di.go is EXPECTED to fail digen generation (digen must
// exit non-zero and print an error describing why generation was rejected).
//
// This test builds a digen binary on the fly into a unique temp path (so it
// never depends on a pre-built bin/digen.exe and never races with other
// threads/processes), then runs it inside every fixture directory and asserts
// the two failure conditions.
package gentest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// fixtureCfg describes the expected failure for a single fixture directory.
//
// flags        : extra digen CLI flags (e.g. "-unused=ignore"); "" means none.
// expectSubstr : a substring that MUST appear in digen's combined
//
//	stdout/stderr. Substrings deliberately avoid absolute paths and
//	version-dependent timestamps so the test stays portable.
type fixtureCfg struct {
	flags        string
	expectSubstr string
}

// fixtures maps a fixture directory name (under example/gen_failures/) to its
// expected failure configuration.
//
// Every subdirectory of example/gen_failures/ MUST have an entry here; the test
// fails if a directory exists without a matching entry (keeps coverage complete
// and self-documenting). To add a new failing fixture: create the directory
// with its di.go, then add an entry below with the expected error substring.
var fixtures = map[string]fixtureCfg{
	"ambiguous":                {"", `with name "s" required by`},
	"capture_const":            {"", `cannot capture local variable "maxRetries"`},
	"capture_ctx":              {"", `cannot capture context variable "globalCtx"`},
	"closure_capture":          {"", `cannot capture local variable "cfg"`},
	"closure_private_fn":       {"", `func "buildAuditAuthorizer" is private in package`},
	"control_flow":             {"", `contains dig.Module inside control flow`},
	"cycle":                    {"", `circular dependency detected`},
	"duplicate_named":          {"", `duplicate binding for`},
	"duplicate_param":          {"", `duplicate parameter type "string"`},
	"duplicate_provide":        {"", `duplicate provide for type "string"`},
	"duplicate_supply":         {"", `duplicate binding for`},
	"init_named_return":        {"", `named return value is not allowed`},
	"invalid_option":           {"", `invalid option expression`},
	"invoke_bad_return":        {"", `single return value must be error`},
	"invoke_option":            {"", `dig.Invoke cannot accept another Option`},
	"invoke_too_many_returns":  {"", `has 2 return values (only 0 or error allowed)`},
	"missing_provider":         {"", `with name "missing" required by`},
	"multiple_build":           {"", `multiple functions containing dig.Build call found`},
	"named_mismatch":           {"", `with name "secondary" required by`},
	"no_build":                 {"", `no packages with dig.Build found`},
	"private_visibility":       {"", `func "newHidden" is private in package`},
	"provide_bad_error":        {"", `second return value must be error`},
	"provide_no_return":        {"", `anonymous provide function has no return`},
	"provide_option":           {"", `dig.Provide cannot accept another Option`},
	"provide_too_many_returns": {"", `too many return values (3)`},
	"provider_ctx":             {"", `declares a context.Context parameter "ctx"`},
	"supply_option":            {"", `dig.Supply cannot accept another Option`},
	"unused_provider":          {"", `unused provider check:`},

	// 以下为 TDD 补全的、此前缺失对应用例的错误分支夹具
	"missing_build_tag": {"", `file contains dig.Build(...) but is missing`},
	"no_module":         {"", `does not contain dig.Module`},
	"init_no_return":    {"", `must have a return value of type func(context.Context) error`},
	"init_multi_return": {"", `only a single return value allowed`},
	"init_bad_return":   {"", `invalid return type "int", expected func(context.Context) error`},

	// 契约预检：用户在 //go:build digen 文件中定义类型/构造器却被接线引用
	"contract_digen_symbol": {"", `digen contract violation`},
}

// repoRoot returns the dig module root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// thisFile = <root>/example/gen_failures/gentest/gen_failures_test.go
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return root
}

// buildDigen compiles the digen binary into a unique temp path and returns the
// path to the executable. It is invoked once per test binary (via TestMain).
func buildDigen(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "digen-gentest-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	binPath := filepath.Join(tmpDir, "digen"+suffix)

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/digen")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build digen: %v\n%s", err, out)
	}
	return binPath
}

// TestGenFailures runs digen against every fixture under example/gen_failures/
// and asserts each one fails generation with its expected error substring.
func TestGenFailures(t *testing.T) {
	binPath := buildDigen(t)
	root := repoRoot(t)
	failuresDir := filepath.Join(root, "example", "gen_failures")

	// Sanity: every configured fixture must actually exist on disk.
	for name := range fixtures {
		if fi, err := os.Stat(filepath.Join(failuresDir, name)); err != nil || !fi.IsDir() {
			t.Errorf("configured fixture %q has no matching directory under example/gen_failures/", name)
		}
	}

	// Reverse sanity: every fixture directory on disk MUST have a matching
	// entry in the fixtures map. Without this, a new failing fixture could be
	// added and silently skipped (the test would still pass), defeating the
	// "coverage is complete and self-documenting" guarantee.
	entries, err := os.ReadDir(failuresDir)
	if err != nil {
		t.Fatalf("read gen_failures dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "gentest" {
			continue
		}
		if _, ok := fixtures[e.Name()]; !ok {
			t.Errorf("fixture directory %q under example/gen_failures/ has no entry in the fixtures map (add one with the expected error substring)", e.Name())
		}
	}

	// Run in a deterministic (sorted) order for stable output.
	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		name := name
		cfg := fixtures[name]
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(failuresDir, name)

			args := []string{}
			if cfg.flags != "" {
				args = append(args, strings.Fields(cfg.flags)...)
			}
			cmd := exec.Command(binPath, args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()

			// (1) digen must exit non-zero.
			if err == nil {
				t.Fatalf("digen unexpectedly succeeded (exit 0) in fixture %q\noutput:\n%s", name, out)
			}
			var exitErr *exec.ExitError
			if !asExitError(err, &exitErr) {
				t.Fatalf("digen failed to run in fixture %q: %v\noutput:\n%s", name, err, out)
			}
			if exitErr.ExitCode() == 0 {
				t.Fatalf("digen exited 0 in fixture %q (expected non-zero)\noutput:\n%s", name, out)
			}

			// (2) the expected error substring must be present.
			if !strings.Contains(string(out), cfg.expectSubstr) {
				t.Fatalf("fixture %q: expected error substring %q not found in digen output:\n%s",
					name, cfg.expectSubstr, out)
			}
		})
	}
}

// asExitError reports whether err is (or wraps) an *exec.ExitError.
func asExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}
