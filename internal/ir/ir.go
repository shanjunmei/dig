// Package ir persists the extractor's output (the stable intermediate
// representation) to disk so that repeated digen runs over an unchanged
// package can skip the expensive extraction / type-checking step.
//
// The on-disk format is JSON (one file per cache key) for debuggability and
// cross-language friendliness. The schema is versioned via model.SchemaVer,
// so a bump in the IR layout automatically invalidates stale cache files
// instead of being silently misinterpreted.
//
// The cache key (see processor.cacheKey) is derived from the config knobs that
// affect the IR (alias style, closure inlining), the Go toolchain version, the
// byte content of the package's own source files, AND the byte content of every
// transitively imported package's source files. Because dependency sources are
// part of the key, a breaking change in an imported package invalidates the
// cache entry automatically — the cache can never serve stale IR that would
// generate code against an old dependency API. (Stdlib packages carry no
// sources to hash, so their API is covered by the Go version in the key.)
package ir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shanjunmei/dig/internal/model"
)

// DefaultCacheDir returns the directory used for IR caches when the caller
// does not supply one. It lives under the OS temp dir so it is per-user and
// cleared on reboot.
func DefaultCacheDir() string {
	return filepath.Join(os.TempDir(), "digen-ir-cache")
}

func entryPath(cacheDir, key string) string {
	return filepath.Join(cacheDir, key+".json")
}

// Load reads a cached extraction for the given key. It returns (nil, false, nil)
// when there is no entry (cache miss). A decoding error is returned as the
// third value so the caller can decide whether to fall back to re-extraction.
func Load(cacheDir, key string) (*model.CachedExtraction, bool, error) {
	data, err := os.ReadFile(entryPath(cacheDir, key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("ir: read cache entry: %w", err)
	}
	var c model.CachedExtraction
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false, fmt.Errorf("ir: decode cache entry (stale/corrupt): %w", err)
	}
	return &c, true, nil
}

// Save writes a cached extraction for the given key. The write is atomic: a
// temp file is written next to the target and renamed into place, so a crashed
// digen run can never leave a half-written cache entry behind.
func Save(cacheDir, key string, c *model.CachedExtraction) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("ir: create cache dir: %w", err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("ir: encode cache entry: %w", err)
	}
	tmp, err := os.CreateTemp(cacheDir, key+".*.tmp")
	if err != nil {
		return fmt.Errorf("ir: create temp cache file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("ir: write temp cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("ir: close temp cache file: %w", err)
	}
	if err := os.Rename(tmpName, entryPath(cacheDir, key)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("ir: rename cache file: %w", err)
	}
	return nil
}
