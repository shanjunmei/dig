# dig — Compile‑time Dependency Injection for Go

[中文文档](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Version**: v1.0.5 – `InitApp()` returns `func(context.Context) error`; generated code has **zero runtime dependency** on `dig`.  
> **Upgrade from v1.0.4**: replace `app.Run(ctx)` with `run := InitApp(); run(ctx)`.

---

## Why dig?

Go DI tools fall into two camps:

- **Uber Fx**: elegant API (`Provide`/`Invoke`/`Supply`/`Module`) but **runtime reflection** – slower startup, runtime panics on dependency errors, larger binaries.
- **Google Wire**: compile‑time safety and zero overhead, but **API is verbose and counter‑intuitive** – repetitive `wire.NewSet`, manual interface binding, `wire.Value` limited to compile‑time constants, and the infamous `wire.Build` dummy `return nil, nil` marker.

**dig** combines the best of both: **Fx‑style minimal API** + **Wire‑style code generation** (no reflection, zero runtime dependency), plus strict closure‑capture safety, generic support, built‑in `Invoke`, and sensible policies for unused providers.

---

## Core Features

- **Compile‑time resolution** – graph resolved during `go generate`; errors are caught at generation time.
- **Zero runtime reflection & zero runtime dependency** – generated code is plain Go, imports nothing.
- **Minimal API** – just `Build`, `Provide`, `Supply`, `Invoke`, `Module`.
- **Closure capture safety** – inline closures cannot capture locals from `InitApp`; enforced by generator.
- **Generic‑aware** – supports generic functions and types natively.
- **Observability** – debug logging with runtime‑overridable `Logf`.
- **Unused‑provider policies** – `error` (default), `ignore`, or `drop`.
- **Module nesting** – compose modules hierarchically; duplicate detection built‑in.

---

## Installation

```bash
go get github.com/shanjunmei/dig@v1.0.8
go install github.com/shanjunmei/dig/cmd/digen@latest
```
Requires Go 1.21+.

---

## Quick Start

**di.go** (build tag `//go:build digen`):
```go
//go:build digen
package main

import (
    "context"
    "github.com/shanjunmei/dig"
)

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out di_gen.go

func InitApp() func(context.Context) error {
    return dig.Build(
        dig.Provide(NewConfig),
        dig.Provide(NewDB),
        dig.Supply(DefaultTimeout),          // direct value
        dig.Provide(func(t Timeout) *Server { return NewServer(t) }),
        dig.Invoke(func(srv *Server) error { return srv.Run() }),
    )
}
```

**main.go** (business logic):
```go
package main

import "context"

type Config struct{ Addr string }
func NewConfig() *Config { return &Config{Addr: ":8080"} }

type DB struct{}
func NewDB(*Config) *DB { return &DB{} }

type Timeout int
var DefaultTimeout Timeout = 5

type Server struct{}
func NewServer(Timeout) *Server { return &Server{} }
func (*Server) Run() error { return nil }

func main() {
    if err := InitApp()(context.Background()); err != nil {
        panic(err)
    }
}
```

**Generate & run**:
```bash
digen ./...   # or go generate ./...
go run .
```

---

## Core API

| Function | Purpose |
|----------|---------|
| `dig.Build(...Option) func(context.Context) error` | Assemble container; returns runnable function. |
| `dig.Provide(any) Option` | Register a constructor (returns a value). |
| `dig.Supply(any) Option` | Inject an existing value (any expression, runtime‑safe). |
| `dig.Invoke(any) Option` | Run a function after all providers are ready (may return error). |
| `dig.Module(...Option) Option` | Group options into reusable, nestable module. |

---

## Key Constraints

### 1. Closure Capture Restriction
Closures inside `Provide`/`Invoke` **cannot capture local variables** from `InitApp` – only package‑level symbols and literals are allowed (generator lifts them to package level).  
✅ Allowed: `func() Timeout { return DefaultTimeout }`  
❌ Forbidden: `t := 5; func() Timeout { return Timeout(t) }`

### 2. External Parameters (InitApp args)
All `InitApp` parameters are automatically registered as `Supply` providers – inject them anywhere.

### 3. Wrapper Types for Primitive Conflicts
Use distinct types to avoid duplicate provider errors for same underlying type (e.g., multiple `bool`s):
```go
type UseMySQL bool
type UseRedis bool
```

### 4. Generics
Explicitly instantiate generic types/functions:
```go
dig.Provide(NewStore[int])
dig.Invoke(Process[string])
```

### 5. Conditional Logic
Branching works **inside** closures (runtime). For compile‑time selection, use build tags – do **not** put conditionals inside `Module()` (all branches are parsed).

### 6. Observability
Run `digen -debug` to inject `Logf` calls. Override at runtime:
```go
var Logf = log.Printf   // defined in di_gen.go
func main() { Logf = myLogger.Printf }
```

### 7. Unused Providers
`-unused=error|ignore|drop` (default `error`).

### 8. Package Aliases
`-alias=full|short|obfuscated` controls generated import aliases.

---

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | `di_gen.go` | Output filename (ignored in `./...` mode) |
| `-unused` | `error` | Policy for unused providers |
| `-debug` | `false` | Enable debug logging |
| `-alias` | `full` | Import alias strategy |

---

## Comparison Matrix

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| **Approach** | Code generation | Code generation | Runtime reflection |
| Zero reflection | ✅ | ✅ | ❌ |
| Zero runtime dependency | ✅ | ✅ | ❌ (needs fx runtime) |
| Validation timing | Generation | Generation | Runtime (panic) |
| **Direct value injection** | ✅ `dig.Supply` (any expr) | ⚠️ `wire.Value` (const‑only, verbose) | ✅ `fx.Supply` |
| Closure capture safety | ✅ enforced | ❌ (silently breaks) | N/A |
| Built‑in `Invoke` | ✅ | ❌ | ✅ |
| Module definition | `func Module() dig.Option` | `var Set = wire.NewSet(...)` | `fx.Module("name", ...)` |
| Module nesting | ✅ explicit | ⚠️ set composition (flat) | ✅ explicit, with naming |
| Generic support | ✅ compile‑time | ⚠️ explicit, messy | ✅ reflection |
| Unused provider policies | 3 modes | only `drop` | N/A |
| Debug logging | ✅ (runtime override) | ❌ manual | ⚠️ tracing (not debug) |
| API ergonomics | Fx‑style, minimal | Wire‑style, verbose & counter‑intuitive | Fx‑style, minimal |
| Refactoring friendliness | High (static checks) | Low (cryptic errors) | Medium (runtime errors) |

> **Wire specifics**: `wire.Build` requires dummy `return nil, nil`; `wire.Value` only works with constants; `wire.NewSet` composition is flat, not nested.

---

## API Quick Migration Reference

| Operation | dig | Wire | Fx |
|-----------|-----|------|----|
| Constructor | `dig.Provide(NewSvc)` | `wire.NewSet(NewSvc)` | `fx.Provide(NewSvc)` |
| Value injection | `dig.Supply(val)` | `wire.Value(val)` (const‑only) | `fx.Supply(val)` |
| Startup hook | `dig.Invoke(fn)` | not built‑in | `fx.Invoke(fn)` |
| Module group | `dig.Module(a, b)` | `wire.NewSet(a, b)` | `fx.Module("name", a, b)` |
| Build container | `dig.Build(...)` (returns runnable) | `wire.Build(...)` (dummy marker) | `fx.New(...)` |
| Run | `run := InitApp(); run(ctx)` | call generated function | `app.Run(ctx)` |

---

## Complete Example

See [`example/`](./example) for a full demonstration covering cross‑package deps, generics, same‑name modules, nesting, external params, `Supply`, closures, debug logs, build tags, and alias strategies.

```bash
cd example
go generate ./...
go run .
```

---

## License

MIT – see [LICENSE](./LICENSE).
