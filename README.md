# dig — Code-Generation Zero-Reflection DI Container for Go
[&#20013;&#25991;](README_zh.md) | **English**

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**dig** is a compile-time code generation dependency injection container for Go.
It eliminates runtime reflection overhead entirely, validates the dependency graph at code generation stage, and provides only four minimal core APIs: `Provide`, `Invoke`, `Supply`, `Module`, following idiomatic Go patterns.

Core design philosophy:
> Static safety at generation time, zero overhead at runtime, no magic, stay native Go.

## Core Advantages
1. **Zero runtime reflection**
All dependency resolution logic is generated as plain Go code via `digen`. There is no runtime type inspection, and startup performance matches handwritten initialization logic.
2. **Strict file isolation via build tags (critical design)**
Split code into 3 independent files with mutually exclusive build tags to avoid duplicate definition / undefined symbol errors:
   - `di.go`: Tag `digen` — Only parsed by digen generator, excluded from normal build
   - `main.go`: No build tag — Shared base types, global variables, business model definitions, compiled in all scenarios
   - `di_gen.go`: Tag `!digen` — Auto-generated placeholder & runtime entry, excluded during code generation
3. **Clear separation between `Supply` and `Provide` closure rules (core constraint)**
   - `dig.Supply`: Accepts package-level global runtime variables
   - Inline `dig.Provide(func() T)` closures: Only constant literals allowed; **cannot capture local variables inside `InitApp()`**, will cause `undefined` compile errors after generation
4. **Custom wrapper types resolve primitive type collision**
Multiple raw `bool` / `time.Duration` cannot be supplied directly; define unique alias types for each business flag to eliminate ambiguous provider errors.
5. **Built-in Invoke startup callback**
Register functions to run automatically after all dependencies are ready for server startup, resource warmup and graceful initialization logic.
6. **Faster code generation than Wire**
The `digen` CLI uses lightweight custom AST parsing, avoiding Wire’s heavy recursive ProviderSet traversal. Generation time is noticeably shorter for mid/large projects with dozens of services.
7. **Lightweight dependency footprint**
Business code only relies on the Go standard library. The CLI generator only depends on official `golang.org/x/tools` for AST analysis, no heavy third-party dependencies.
8. **Configurable unused constructor policy**
Three strategies for handling unused constructors:
- `error`: Fail code generation, force cleanup of redundant constructors
- `ignore`: Suppress warnings silently
- `drop`: Remove unused code from the generated file

## Mandatory Build Tag Isolation Rules (Must Follow)
### File Split & Tag Matching
1. `di.go` (Generator-only init logic)
   Build tag: `//go:build digen`
   - Contains only `InitApp()` with `dig.Build()` chain
   - Parsed exclusively by `digen` during `go generate`
   - Skipped when building/running normally (tag mismatch)
2. `main.go` (Shared base code, no build tag)
   - All custom wrapper types, model structs, global runtime variables, `main()` entry
   - Compiled in all build modes, visible to both generator and runtime
3. `di_gen.go` (Auto-generated runtime entry)
   Build tag: `//go:build !digen`
   - Auto-overwritten by digen after generate
   - Provides fallback `InitApp()` for normal runtime execution
   - Skipped during code generation (tag `!digen` conflicts with generator `--tags=digen`)

### Why Isolation Is Required
If files share the same build tag or omit tags incorrectly:
- Duplicate definition error: Two `InitApp()` functions exist in same package
- Undefined symbol error: Generator cannot access base types / global variables
- Broken dependency topology parsing in digen

## Installation
### Step 1: Add library dependency to your project
```bash
go get github.com/shanjunmei/dig
```
### Step 2: Install code generation CLI tool
```bash
go install github.com/shanjunmei/dig/cmd/digen@latest
```
Requires Go 1.21+.

## Complete Standard Demo (3 Separated Files)
### 1. di.go (Generator Only, tag digen)
```go
//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out di_gen.go -unused=drop --tags=digen
//go:build digen
// +build digen
package main

import (
	"time"
	"github.com/shanjunmei/dig"
)

func InitApp() *dig.App {
	return dig.Build(
		// Supply accepts package-level global variable defined in main.go
		dig.Supply(s),

		// Provide closure only uses constant literals, no outer variable capture
		dig.Provide(func() EnableCache { return EnableCache(true) }),
		dig.Provide(func() QueryTimeout { return QueryTimeout(3 * time.Second) }),

		dig.Provide(NewConfig),

		// All dependencies come from function input params, no local capture
		dig.Provide(func(t QueryTimeout, um UseMySQL, rm EnableCache) any {
			if um {
				return NewMySQLStore(t)
			}
			return NewRedisStore(rm)
		}),

		dig.Provide(NewServer),

		// Invoke only consumes injected parameters
		dig.Invoke(func(srv *Server, um UseMySQL, ec EnableCache, t QueryTimeout) {
			println("service launch")
			println("use_mysql:", bool(um))
			println("cache_enabled:", bool(ec))
			println("timeout sec:", time.Duration(t).Seconds())
			_ = srv.Run()
		}),
	)
}
```

### 2. main.go (Shared Base Code, No Build Tag)
```go
package main

import (
	"context"
	"time"
)

// Package global runtime variable, consumed by dig.Supply
var s = UseMySQL(true)

// Unique wrapper types to avoid primitive type collision
type UseMySQL bool
type EnableCache bool
type QueryTimeout time.Duration

// Business models & constructors
type Config struct {
	Addr string
}
func NewConfig() *Config {
	return &Config{Addr: "0.0.0.0:8080"}
}

type MySQLStore struct{}
func NewMySQLStore(t QueryTimeout) *MySQLStore {
	return &MySQLStore{}
}

type RedisStore struct{}
func NewRedisStore(c EnableCache) *RedisStore {
	return &RedisStore{}
}

type Server struct {
	store any
}
func NewServer(s any) *Server {
	return &Server{store: s}
}
func (s *Server) Run() error { return nil }

func main() {
	app := InitApp()
	_ = app.Run(context.Background())
}
```

### 3. di_gen.go (Auto-Generated Runtime Entry, tag !digen)
```go
// Code generated by digen; DO NOT EDIT.
//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out di_gen.go -unused=drop --tags=digen
//go:build !digen
// +build !digen

package main

import (
	"context"
	"github.com/shanjunmei/dig"
	"time"
)

// Auto-split provider & invoke functions generated by digen
func __p_1() EnableCache {
	return EnableCache(true)
}
func __p_2() QueryTimeout {
	return QueryTimeout(3 * time.Second)
}
func __p_4(t QueryTimeout, um UseMySQL, rm EnableCache) any {
	if um {
		return NewMySQLStore(t)
	}
	return NewRedisStore(rm)
}
func __i_6(srv *Server, um UseMySQL, ec EnableCache, t QueryTimeout) {
	println("service launch")
	println("use_mysql:", bool(um))
	println("cache_enabled:", bool(ec))
	println("timeout sec:", time.Duration(t).Seconds())
	_ = srv.Run()
}

// Runtime fallback InitApp, only compiled under !digen tag
func InitApp() *dig.App {
	v0 := s
	v1 := __p_1()
	v2 := __p_2()
	v4 := __p_4(v2, v0, v1)
	v5 := NewServer(v4)
	return dig.New(func(ctx context.Context) error {
		__i_6(v5, v0, v1, v2)
		return nil
	})
}
```

## Generate & Run Commands
```bash
# Generate DI code
go generate ./...
# Or direct digen call
digen -out di_gen.go -unused=drop --tags=digen

# Normal runtime build & run
go run .
```

## Comparison with Mainstream DI Frameworks
| Comparison Item             | dig (This Project) | Google Wire          | Uber dig             | Uber FX              |
| --------------------------- | ------------------ | -------------------- | -------------------- | -------------------- |
| Code generation required    | &#9989; digen CLI        | &#9989; wire gen           | &#10060; No                | &#10060; No                |
| Runtime reflection          | &#10060; None at all      | &#10060; None at all       | &#9989; Heavy reflection  | &#9989; Heavy reflection  |
| Generation-time full validation | &#9989; Complete | &#9989; Complete | &#10060; Only runtime check | &#10060; Only runtime check |
| Mutually exclusive build tag file split | &#9989; Official standard | &#9888;&#65039; Not supported | &#10060; Not supported | &#10060; Not supported |
| Supply supports global runtime variables | &#9989; Native | &#9888;&#65039; Limited | &#10060; Not supported | &#10060; Not supported |
| Provide closure only constant literals | &#9989; Enforced rule | &#9888;&#65039; Unrestricted capture (scope loss risk) | &#10060; No closure support | &#10060; No closure support |
| Wrapper type resolve primitive collision | &#9989; Official recommended | &#9888;&#65039; Manual workaround | &#10060; Not supported | &#10060; Not supported |
| Built-in Invoke startup callback | &#9989; Native | &#10060; Need manual boilerplate | &#10060; Not supported | &#9989; Built-in lifecycle |
| Unused constructor control  | 3 configurable modes | Silent drop only    | Silent ignore        | Silent ignore        |
| Business code external deps | Zero (stdlib only) | Zero                 | Multiple             | Multiple heavy deps  |
| Learning curve              | Very low (4 core APIs + strict tag/supply rules) | Medium verbose syntax | Medium tag & scope rules | High complex lifecycle system |
| Final binary size           | Tiny, no reflection logic | Tiny | Medium | Largest (embed log/lifecycle/event bus) |
| Best application scenario   | Projects pursue zero runtime overhead & static safety, multi-file tag isolation, dynamic env flags via Supply | Legacy simple services with strict third-party limits, no tag isolation / runtime variable injection demand | Tiny simple CLI tools, tolerate reflection cost | Large microservices need full lifecycle & plugin ecosystem |

## CLI Flags for digen
| Flag      | Description |
| --------- | ----------- |
| `-out`    | Output generated file path, default `dig_gen.go` |
| `-unused` | Policy for unused constructors: `error` / `ignore` / `drop` |
| `--tags`  | Build tags passed to Go AST parser, exclude runtime generated file during generation |
| `-debug`  | Print AST parsing debug logs to stdout |

## Lint Standard
The project provides cross-platform `lint.ps1` static check script aligned with Go Report Card standard:
1. `gofmt -s -l` format verification
2. `go vet` official static analysis
3. `gocyclo -over 15` cyclomatic complexity limit
4. `ineffassign` unused variable detection
5. `misspell` English spelling check

Run lint on Windows PowerShell:
```powershell
.\lint.ps1
```

## License
MIT License, see `LICENSE` file in repository root.
