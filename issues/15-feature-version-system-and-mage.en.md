# Feature: Version Info System and Mage Build Support

## Description

Previously, digen had no version info system. Users couldn't check the current version via a `-version` flag. The build process had no automation and required manual `go build` execution. Additionally, binaries installed via `go install` couldn't automatically obtain version information.

## Solution

### 1. New Version Info Module (`cmd/digen/version.go`)

Implemented a flexible version info system with multiple info sources and priority handling:

**Version Info Source Priority**:
1. **ldflags injection** (highest): injected by build tools via `-ldflags "-X main.Version=v1.0.0 ..."`
2. **VCS info fallback**: when ldflags are empty, automatically reads Go-embedded VCS info from `debug.ReadBuildInfo()`

**Version String Format**:
```
digen v1.0.11+13 (dirty)
  commit: 0f16d86
  built:  2026-07-29 18:48:05 -07:00
```

**git describe Format Parsing**:
| git describe output | Parsed result |
|---|---|
| `v1.0.11` | tag=v1.0.11 |
| `v1.0.11-dirty` | tag=v1.0.11, dirty=true |
| `v1.0.11-13-g0f16d86` | tag=v1.0.11, commits=13, commit=0f16d86 |
| `v1.0.11-13-g0f16d86-dirty` | tag=v1.0.11, commits=13, dirty=true |

**Key Features**:
- No default values: version is not forcibly set, left empty when missing
- Go native support: automatically obtains VCS info for `go install`-installed binaries via `debug.ReadBuildInfo()`
- Caching: `getVersion()` uses package-level cache to avoid repeated parsing
- Multiple date formats: `formatDate` supports RFC3339 and other date format parsing

### 2. New `-version` CLI Flag (`cmd/digen/main.go`)

```go
showVersion := flag.Bool("version", false, "print version information and exit")
// ...
if *showVersion {
    fmt.Println(versionString())
    return
}
```

### 3. New Mage Build System (`magefile.go`)

Provides complete automated build tasks:

| Task | Description |
|---|---|
| `mage build` | Build binary, auto-fetches version from git and injects via ldflags |
| `mage install` | Install to `$GOPATH/bin`, injects version |
| `mage generate` | Run digen to generate example code |
| `mage test` | Run tests (`go test -v -race ./...`) |
| `mage vet` | Static analysis (`go vet ./...`) |
| `mage clean` | Clean `bin/` directory and `*_gen.go` files |
| `mage default` | Default: runs `build` |

**Auto-injection during build**:
```go
func ldflags() string {
    version, commit, buildDate := getVersionInfo()
    return fmt.Sprintf("-X 'main.Version=%s' -X 'main.Commit=%s' -X 'main.BuildDate=%s'",
        version, commit, buildDate)
}
```

**Version info from git**:
- `git describe --tags --dirty`: get version tag with dirty status
- `git rev-parse HEAD`: get full commit hash

### 4. New `.gitignore`

Ignores build artifacts, IDE configs, OS temp files, etc.:
- `bin/`, `*.exe`, `*.test` and other build artifacts
- `.idea/`, `.vscode/` and other IDE configs
- `go.work`, `*.log` and other temp files

## Files Changed

- `cmd/digen/version.go` (new): Version info parsing and formatting
- `cmd/digen/main.go` (modified): Added `-version` flag
- `magefile.go` (new): Mage build system
- `.gitignore` (new): Ignore rules

## Backward Compatibility

- Existing CLI flags remain unchanged
- Without mage, `go build ./cmd/digen` still works fine (just without version info)
- When installed via `go install github.com/shanjunmei/dig/cmd/digen@latest`, Go 1.18+ automatically embeds VCS info, and the version number displays correctly
