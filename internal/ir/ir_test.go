package ir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shanjunmei/dig/internal/model"
)

func TestCacheMissThenHit(t *testing.T) {
	dir := t.TempDir()
	key := "abc123"

	// Clean miss first.
	if _, hit, err := Load(dir, key); err != nil || hit {
		t.Fatalf("expected clean miss, hit=%v err=%v", hit, err)
	}

	entry := &model.CachedExtraction{
		Nodes:          []model.Node{{Name: "n1", Func: "F", RetType: "int"}},
		ImportAliasMap: map[string]string{"fmt": "fmt"},
		PkgAliasMap:    map[string]string{"p": "a"},
		PkgNameMap:     map[string]string{"p": "a"},
	}
	if err := Save(dir, key, entry); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Now a hit, with identical content.
	got, hit, err := Load(dir, key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !hit {
		t.Fatal("expected hit after save")
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Name != "n1" {
		t.Fatalf("nodes mismatch: %+v", got.Nodes)
	}
	if got.SchemaVer != model.SchemaVersion {
		t.Fatalf("schema_ver = %d, want %d", got.SchemaVer, model.SchemaVersion)
	}
	if got.ImportAliasMap["fmt"] != "fmt" {
		t.Fatalf("import map mismatch: %v", got.ImportAliasMap)
	}

	// A different key stays a miss.
	if _, hit, _ := Load(dir, "other"); hit {
		t.Fatal("expected miss for a different key")
	}
}

func TestLoadCorruptReturnsError(t *testing.T) {
	dir := t.TempDir()
	key := "k"
	if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := Load(dir, key); err == nil {
		t.Fatalf("expected error on corrupt file, got hit=%v", hit)
	}
}

func TestSaveCreatesDirAndWritesAtomicFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	key := "k2"
	entry := &model.CachedExtraction{
		Nodes: []model.Node{{Name: "x"}},
	}
	if err := Save(dir, key, entry); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, key+".json")); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}
	// No temp file should linger.
	matches, _ := filepath.Glob(filepath.Join(dir, key+".*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("lingering temp files: %v", matches)
	}
}
