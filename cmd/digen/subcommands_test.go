package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunInit scaffolds a di.go and writes it to the requested path.
func TestRunInit(t *testing.T) {
	name := filepath.Join(t.TempDir(), "di.go")
	if err := runInit([]string{name}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}
	if !strings.Contains(string(data), "dig.Build") {
		t.Fatalf("scaffolded di.go missing dig.Build entry point:\n%s", data)
	}
}

// TestRunInitRefusesOverwrite must not clobber an existing file.
func TestRunInitRefusesOverwrite(t *testing.T) {
	name := filepath.Join(t.TempDir(), "di.go")
	if err := os.WriteFile(name, []byte("// existing"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := runInit([]string{name}); err == nil {
		t.Fatal("expected runInit to refuse overwriting an existing file")
	}
}

// TestRunCheckPasses validates a real dig.Build entry point without failing.
func TestRunCheckPasses(t *testing.T) {
	if err := runCheck(cliFlags{alias: "full", unused: "ignore"}, []string{"github.com/shanjunmei/dig/example/app_basic"}); err != nil {
		t.Fatalf("runCheck on example/app_basic: %v", err)
	}
}

// TestRunCheckNoBuildFails reports a clear error when no package has dig.Build.
func TestRunCheckNoBuildFails(t *testing.T) {
	err := runCheck(cliFlags{alias: "full", unused: "ignore"}, []string{"github.com/shanjunmei/dig/internal/loader"})
	if err == nil {
		t.Fatal("expected runCheck to fail for a package without dig.Build")
	}
	if !strings.Contains(err.Error(), "no packages with dig.Build found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunGraphPasses renders the Mermaid graph for a real entry point.
func TestRunGraphPasses(t *testing.T) {
	if err := runGraph(cliFlags{alias: "full", unused: "ignore"}, []string{"github.com/shanjunmei/dig/example/app_basic"}); err != nil {
		t.Fatalf("runGraph on example/app_basic: %v", err)
	}
}

// TestRunExplainNoProviderFails fails deterministically for an unknown type.
func TestRunExplainNoProviderFails(t *testing.T) {
	err := runExplain(cliFlags{alias: "full", unused: "ignore"}, []string{"NoSuchTypeXYZ", "github.com/shanjunmei/dig/example/app_basic"})
	if err == nil {
		t.Fatal("expected runExplain to fail for an unknown type")
	}
	if !strings.Contains(err.Error(), "no provider found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
