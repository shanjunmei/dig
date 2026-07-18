## LLM Agent Skills
All system prompts for AI assistant are stored in [`prompts`](./prompts)
- [`system_prompt_dig_en.md`](./prompts/system_prompt_dig_en.md): Professional skill for github.com/shanjunmei/dig compile-time DI library
### Official Industrial Modular Coding Skill
A complete standardized production coding convention skill for business microservice based on dig:
[Industrial Modular Coding Skill](./prompts/industrial_modular_coding_skill.md)

# dig — Compile‑time Dependency Injection for Go

[中文文档](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Current version**: v1.0.11
>
> **Key version changes**:
> - **v1.0.11**: Added named instance injection, fixed package alias resolution (e.g. `go-redis/v9`)
> - **v1.0.5**: `InitApp()` returns `func(context.Context) error`; generated code has zero runtime dependency
> - **v1.0.4**: Initial stable release
>
> **Upgrade from v1.0.4**: replace `app.Run(ctx)` with `run := InitApp(); run(ctx)`.

---

## Why dig?

Go DI tools fall into two camps:

- **Uber Fx**: elegant API (`Provide`/`Invoke`/`Supply`/`Module`) but **runtime reflection** – slower startup, runtime panics on dependency errors, larger binaries.
- **Google Wire**: compile‑time safety and zero overhead, but **API is verbose and counter‑intuitive** – repetitive `wire.NewSet`, manual interface binding, `wire.Value` limited to compile‑time constants, and the infamous `wire.Build` dummy `return nil, nil` marker.

**dig** combines the best of both: **Fx‑style minimal API** + **Wire‑style code generation** (no reflection, zero runtime dependency), plus strict closure‑capture safety, generic support, built‑in `Invoke`, sensible policies for unused providers, and **native support for multiple instances of the same type via parameter names**.

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
- **Named instance injection** – inject multiple instances of the same type by distinguishing them via **parameter names**.

---

## Installation

```bash
go get github.com/shanjunmei/dig@v1.0.11
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

## Named Instance Injection

dig supports injecting multiple instances of the **same type** by differentiating them through **parameter names**. This is useful for scenarios like:

- Multiple database connections (primary, replica, reporting)
- Multiple Redis clients for different business domains
- Multiple HTTP clients with different configurations

### How It Works

1. **Define a provider with named return values** – the names become the "instance names".
2. **Depend on a specific instance** by using the same parameter name in your consumer function.

### Example

```go
// Provider returns two *sql.DB instances with different names
dig.Provide(func() (mainDB *sql.DB, reportDB *sql.DB, error) {
    main, err := connectMain()
    if err != nil { return nil, nil, err }
    report, err := connectReport()
    if err != nil { return nil, nil, err }
    return main, report, nil
})

// Consumer uses the main database
dig.Invoke(func(mainDB *sql.DB) {
    // mainDB is automatically injected
})

// Consumer uses the report database
dig.Invoke(func(reportDB *sql.DB) {
    // reportDB is automatically injected
})
```

### Using `dig.Supply` with Names

You can also supply named values directly:

```go
dig.Supply(mainDB)   // variable name becomes instance name
dig.Supply(reportDB)
```

The generator uses the **variable name** (not the type) to distinguish instances.

### Error Handling

If multiple instances exist for the same type and a consumer does **not** specify a parameter name, the generator will produce an error listing the available names:

```text
ambiguous dependency: multiple providers for type *sql.DB available:
  - mainDB
  - reportDB
```

### Compatibility

- Existing code that uses a single instance of a type remains unchanged.
- The feature is additive – no breaking changes.

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
| **Multiple instances of same type** | ✅ **Named parameters** | ❌ Not supported (must use wrapper types) | ✅ **Value Groups** |
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

See [`example/`](./example) for a full demonstration covering cross‑package deps, generics, same‑name modules, nesting, external params, `Supply`, closures, debug logs, build tags, alias strategies, and **named instance injection** for multiple databases.

```bash
cd example
digen -unused=ignore ./...
go run .
```

---

## License

MIT – see [LICENSE](./LICENSE).
