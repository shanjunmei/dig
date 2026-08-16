// Package golden provides a golden-file regression test for digen's CODE
// GENERATION output.
//
// For every complex example package under example/ that ships a committed
// dig_gen.go (the golden snapshot), this test:
//
//  1. reads the //go:generate directive embedded in the golden to recover the
//     exact CLI flags used to produce it;
//  2. rebuilds the current digen and regenerates the file into a temp -out
//     (never touching the committed golden);
//  3. normalizes both files (strips the version-independent //go:generate meta
//     line) and diffs them.
//
// Any divergence fails the test. This is the safety net against SILENT
// generation drift: a change to the generator that still compiles and still
// "works" but emits different code (including the dangerous "compiles but the
// wiring is semantically wrong" class) is caught here on the next run, before
// it ever ships. The committed dig_gen.go files are the source of truth; this
// test enforces that the generator keeps reproducing them byte-for-byte.
package golden

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestGoldenFiles regenerates every discovered example golden and asserts it
// matches the committed snapshot.
func TestGoldenFiles(t *testing.T) {
	binPath := buildDigen(t)
	root := repoRoot(t)

	targets := discoverGoldens(t, root)
	if len(targets) == 0 {
		t.Fatal("no golden example packages discovered under example/")
	}
	sort.Strings(targets)

	for _, rel := range targets {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			dir := filepath.Join(root, rel)
			goldenPath := filepath.Join(dir, "dig_gen.go")

			flags, err := parseGenFlags(goldenPath)
			if err != nil {
				t.Fatalf("%s: %v", rel, err)
			}

			tmp, err := os.CreateTemp(t.TempDir(), "dig_gen_*.go")
			if err != nil {
				t.Fatalf("CreateTemp: %v", err)
			}
			tmp.Close()
			genOut := tmp.Name()

			// Rewrite -out to the temp file so the committed golden is untouched.
			args := rewriteOut(flags, genOut)

			cmd := exec.Command(binPath, args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: digen failed to regenerate golden (exit %v):\n%s", rel, err, out)
			}

			genBytes, err := os.ReadFile(genOut)
			if err != nil {
				t.Fatalf("%s: read generated: %v", rel, err)
			}
			goldBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("%s: read golden: %v", rel, err)
			}

			genNorm := normalize(string(genBytes))
			goldNorm := normalize(string(goldBytes))
			if genNorm != goldNorm {
				t.Fatalf("%s: regenerated output differs from the committed golden (dig_gen.go).\n"+
					"This means the generator's output changed. If the change is intentional, "+
					"regenerate the golden with `digen` using the flags recorded in its //go:generate line.\n"+
					"--- first divergence (generated vs golden) ---\n%s",
					rel, firstDiff(genNorm, goldNorm))
			}
		})
	}
}

// discoverGoldens returns the relative paths (to repo root) of example packages
// that ship a committed dig_gen.go with a //go:generate directive. gen_failures
// is excluded (those are EXPECTED to fail generation) and the golden package
// itself is excluded.
func discoverGoldens(t *testing.T, root string) []string {
	exampleDir := filepath.Join(root, "example")
	entries, err := os.ReadDir(exampleDir)
	if err != nil {
		t.Fatalf("read example dir: %v", err)
	}
	var targets []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "gen_failures" || name == "golden" {
			continue
		}
		golden := filepath.Join(exampleDir, name, "dig_gen.go")
		if fi, err := os.Stat(golden); err != nil || fi.IsDir() {
			continue
		}
		if _, err := parseGenFlags(golden); err != nil {
			// No //go:generate directive -> not a regeneration target.
			continue
		}
		targets = append(targets, filepath.Join("example", name))
	}
	return targets
}

// parseGenFlags reads the //go:generate directive from a dig_gen.go and returns
// the digen CLI flags it specifies (everything after "cmd/digen").
func parseGenFlags(goldenPath string) ([]string, error) {
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//go:generate") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(trimmed, "//go:generate"))
		idx := strings.Index(body, "cmd/digen")
		if idx < 0 {
			return nil, fmt.Errorf("//go:generate line does not reference cmd/digen: %q", body)
		}
		rest := body[idx+len("cmd/digen"):]
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return nil, fmt.Errorf("//go:generate line has no digen flags: %q", body)
		}
		return fields, nil
	}
	return nil, fmt.Errorf("no //go:generate directive found in %s", goldenPath)
}

// rewriteOut replaces the -out=... flag with the given path so generation does
// not clobber the committed golden.
func rewriteOut(flags []string, out string) []string {
	outFlag := "-out=" + out
	args := make([]string, len(flags))
	replaced := false
	for i, f := range flags {
		if strings.HasPrefix(f, "-out=") {
			args[i] = outFlag
			replaced = true
		} else {
			args[i] = f
		}
	}
	if !replaced {
		args = append(args, outFlag)
	}
	return args
}

// normalize strips the version-independent //go:generate meta line (and trims
// trailing blank lines) so only the semantically meaningful generated code is
// compared.
func normalize(src string) string {
	var kept []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//go:generate") {
			continue
		}
		kept = append(kept, line)
	}
	// Drop trailing empty lines.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n")
}

// firstDiff returns a short, readable diff of the first divergence between two
// normalized golden strings.
func firstDiff(a, b string) string {
	la := strings.Split(a, "\n")
	lb := strings.Split(b, "\n")
	n := len(la)
	if len(lb) < n {
		n = len(lb)
	}
	var b2 strings.Builder
	for i := 0; i < n; i++ {
		if la[i] != lb[i] {
			b2.WriteString("@@ line " + itoa(i+1) + " @@\n")
			b2.WriteString("- generated: " + la[i] + "\n")
			b2.WriteString("+ golden:    " + lb[i] + "\n")
			// Show a little context.
			if i+1 < len(la) {
				b2.WriteString("  generated(next): " + la[i+1] + "\n")
			}
			if i+1 < len(lb) {
				b2.WriteString("  golden(next):    " + lb[i+1] + "\n")
			}
			break
		}
	}
	if b2.Len() == 0 {
		b2.WriteString("(files differ only in length)\n")
	}
	return b2.String()
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

// repoRoot returns the dig module root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return root
}

// buildDigen compiles the digen binary into a unique temp path.
func buildDigen(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "digen-golden-")
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
