// Package successtest provides a regression guard for the SUCCESS-path
// generation of the example/ packages.
//
// Unlike example/gen_failures/gentest (which asserts digen REJECTS bad input),
// this package asserts digen ACCEPTS every valid example and that the freshly
// regenerated dig_gen.go still compiles.
//
// Why this matters: the committed example/*/dig_gen.go files are frozen
// snapshots. Nothing in the repo (there is no CI regeneration step) re-runs
// digen on them, so a generator regression would sit undetected — go build
// ./... stays green because it compiles the stale committed file. This test
// closes that gap by actually regenerating each example with the current digen
// and verifying the output compiles.
//
// To avoid polluting the working tree, each example's dig_gen.go is restored to
// its original content in a t.Cleanup, even if generation or the build fails.
package successtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// digenFlagsRx extracts the digen-specific flags from a committed dig_gen.go
// header line of the form:
//
//	//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen  -debug=false -unused=ignore -alias=full -inline=true -out=dig_gen.go
//
// Only the flags digen understands are captured (-debug/-unused/-alias/-inline/-out);
// the surrounding "go run -mod=mod <path>" prefix is ignored.
var digenFlagsRx = regexp.MustCompile(`-(debug|unused|alias|inline|out)=\S+`)

// fixture describes one success example package to regenerate.
type fixture struct {
	dir   string // package directory under the module root, e.g. example/app
	flags []string
}

// discoverSuccessFixtures scans example/ for packages that contain both a
// di.go (the //go:build digen DI spec) and a dig_gen.go (generated output),
// excluding the gen_failures tree. For each it reads the committed dig_gen.go
// header to recover the exact flags the author used to generate it.
func discoverSuccessFixtures(t *testing.T, root string) []fixture {
	t.Helper()
	exampleDir := filepath.Join(root, "example")
	entries, err := os.ReadDir(exampleDir)
	if err != nil {
		t.Fatalf("read example dir: %v", err)
	}
	var fix []fixture
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "gen_failures" {
			continue
		}
		dir := filepath.Join("example", e.Name())
		diPath := filepath.Join(root, dir, "di.go")
		genPath := filepath.Join(root, dir, "dig_gen.go")
		if !fileExists(diPath) || !fileExists(genPath) {
			continue
		}
		fix = append(fix, fixture{
			dir:   dir,
			flags: parseDigenFlags(t, genPath),
		})
	}
	if len(fix) == 0 {
		t.Fatal("discovered zero success fixtures under example/")
	}
	return fix
}

// parseDigenFlags reads the //go:generate header line of a committed dig_gen.go
// and returns the digen-specific flags. Falls back to the canonical example
// flags if the header cannot be parsed.
func parseDigenFlags(t *testing.T, genPath string) []string {
	t.Helper()
	data, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("read %s: %v", genPath, err)
	}
	header := strings.SplitN(string(data), "\n", 8)
	for _, line := range header {
		if strings.Contains(line, "go:generate") {
			matches := digenFlagsRx.FindAllString(line, -1)
			if len(matches) > 0 {
				return matches
			}
		}
	}
	// Fallback: canonical example flags.
	return []string{"-debug=false", "-unused=ignore", "-alias=full", "-inline=true", "-out=dig_gen.go"}
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// repoRoot returns the dig module root by walking up from this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// thisFile = <root>/example/successtest/success_test.go
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
	tmpDir, err := os.MkdirTemp("", "digen-successtest-")
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

// TestSuccessGen regenerates every success example with the current digen and
// asserts (1) digen exits 0 and (2) the regenerated dig_gen.go compiles via
// `go build`. The example's dig_gen.go is restored afterwards.
func TestSuccessGen(t *testing.T) {
	binPath := buildDigen(t)
	root := repoRoot(t)
	fixtures := discoverSuccessFixtures(t, root)

	names := make([]string, 0, len(fixtures))
	for _, f := range fixtures {
		names = append(names, f.dir)
	}
	sort.Strings(names)

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(root, name)
			genPath := filepath.Join(dir, "dig_gen.go")

			// Snapshot and restore so the working tree is left untouched.
			orig, err := os.ReadFile(genPath)
			if err != nil {
				t.Fatalf("read original %s: %v", genPath, err)
			}
			t.Cleanup(func() { _ = os.WriteFile(genPath, orig, 0644) })

			// Recover the flags the author used for this example.
			flags := parseDigenFlags(t, genPath)

			// (1) digen must regenerate successfully (exit 0).
			cmd := exec.Command(binPath, flags...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("digen failed to regenerate %q: %v\noutput:\n%s", name, err, out)
			}

			// (2) the regenerated dig_gen.go must compile.
			build := exec.Command("go", "build", "./"+name)
			build.Dir = root
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("regenerated %q did not compile: %v\noutput:\n%s", name, err, out)
			}
		})
	}
}
