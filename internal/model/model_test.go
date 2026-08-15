package model

import (
	"encoding/json"
	"testing"
)

func sampleExtraction() *CachedExtraction {
	return &CachedExtraction{
		Nodes: []Node{
			{
				Name:      "provideFoo",
				Func:      "NewFoo",
				FuncPkg:   "example/app",
				RetType:   "*Foo",
				Args:      []Arg{{Name: "cfg", IsConst: false}},
				HasError:  false,
				IsClosure: false,
				UsedPkgs:  []string{"fmt"},
				PkgPath:   "example/app",
				Comment:   "provides a foo",
				Position:  "app.go:42:6",
			},
		},
		ImportAliasMap: map[string]string{"fmt": "fmt"},
		PkgAliasMap:    map[string]string{"example/app": "app"},
		PkgNameMap:     map[string]string{"example/app": "app"},
	}
}

func TestCachedExtractionJSONRoundTrip(t *testing.T) {
	c := sampleExtraction()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// schema version must have been stamped on the bundle and every node.
	var bundle map[string]any
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("decode check: %v", err)
	}
	if int(bundle["schema_ver"].(float64)) != SchemaVersion {
		t.Fatalf("bundle schema_ver not stamped: %v", bundle["schema_ver"])
	}

	var got CachedExtraction
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVer != SchemaVersion {
		t.Fatalf("loaded schema_ver = %d, want %d", got.SchemaVer, SchemaVersion)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("nodes len = %d", len(got.Nodes))
	}
	if got.Nodes[0].SchemaVer != SchemaVersion {
		t.Fatalf("node schema_ver not stamped: %d", got.Nodes[0].SchemaVer)
	}
	if got.Nodes[0].Name != "provideFoo" {
		t.Fatalf("node name = %q", got.Nodes[0].Name)
	}
	if got.ImportAliasMap["fmt"] != "fmt" {
		t.Fatalf("import alias map mismatch: %v", got.ImportAliasMap)
	}
	if got.PkgAliasMap["example/app"] != "app" {
		t.Fatalf("pkg alias map mismatch: %v", got.PkgAliasMap)
	}
	if got.PkgNameMap["example/app"] != "app" {
		t.Fatalf("pkg name map mismatch: %v", got.PkgNameMap)
	}

	// MarshalJSON must copy nodes, not mutate the caller's slice elements.
	if c.Nodes[0].SchemaVer != 0 {
		t.Fatalf("marshal mutated caller node schema_ver = %d (must not mutate caller)", c.Nodes[0].SchemaVer)
	}
}

func TestCachedExtractionSchemaMismatch(t *testing.T) {
	bad := []byte(`{"schema_ver":99,"nodes":[],"import_alias_map":{},"pkg_alias_map":{},"pkg_name_map":{}}`)
	var c CachedExtraction
	if err := json.Unmarshal(bad, &c); err == nil {
		t.Fatal("expected schema version mismatch error, got nil")
	}
}

func TestCachedExtractionGobRoundTrip(t *testing.T) {
	c := sampleExtraction()
	data, err := c.EncodeGob()
	if err != nil {
		t.Fatalf("encode gob: %v", err)
	}
	var got CachedExtraction
	if err := got.DecodeGob(data); err != nil {
		t.Fatalf("decode gob: %v", err)
	}
	if got.SchemaVer != SchemaVersion {
		t.Fatalf("gob schema_ver = %d, want %d", got.SchemaVer, SchemaVersion)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Name != "provideFoo" {
		t.Fatalf("gob nodes round trip failed: %+v", got.Nodes)
	}
	if got.ImportAliasMap["fmt"] != "fmt" {
		t.Fatalf("gob import map failed: %v", got.ImportAliasMap)
	}
	if got.PkgAliasMap["example/app"] != "app" {
		t.Fatalf("gob pkg alias map failed: %v", got.PkgAliasMap)
	}
}
