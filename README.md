## LLM Agent Skills
Optimized AI assistant prompts for dig live in [`prompts`](./prompts). Entry point: [`system_prompt_dig_en.md`](./prompts/system_prompt_dig_en.md) — covers core API & CLI, troubleshooting, version migration, and a dig/Wire/Fx comparison.
### Official Industrial Modular Coding Skill
A complete standardized production coding convention skill for business microservice based on dig:
[Industrial Modular Coding Skill](./prompts/industrial_modular_coding_skill.md)

# dig — Compile‑time Dependency Injection for Go

[中文文档](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Current version**: v1.0.18 — full release notes in [CHANGELOG_en.md](./CHANGELOG_en.md).

---

## Why dig?

Go DI tools fall into two camps:

- **Uber Fx**: elegant API (`Provide`/`Invoke`/`Supply`/`Module`) but **runtime reflection** – slower startup, runtime panics on dependency errors, larger binaries.
- **Google Wire**: compile‑time safety and zero overhead, but **API is verbose and counter‑intuitive** – repetitive `wire.NewSet`, manual interface binding, `wire.Value` forbids function calls and channel receives, and the infamous `wire.Build` dummy `return nil, nil` marker.

**dig** combines the best of both: **Fx‑style minimal API** + **Wire‑style code generation** (no reflection, zero runtime dependency), plus strict closure‑capture safety, generic support, built‑in `Invoke`, sensible policies for unused providers, and **native support for multiple instances of the same type via parameter names**.

> **Note on the name:** This project (`github.com/shanjunmei/dig`) is a *compile‑time, code‑generation* DI library and is **unrelated** to Uber's runtime reflection container `go.uber.org/dig` (the engine behind `uber‑go/fx`). Don't confuse the two.

---

## Core Features

- **Compile‑time resolution** – graph resolved during `go generate`; errors are caught at generation time.
- **Zero runtime reflection & zero runtime dependency** – generated code is plain Go with no third-party runtime dependency (only the standard library, e.g. `context`).
- **Minimal API** – just `Build`, `Provide`, `Supply`, `Invoke`, `Module`.
- **Closure capture safety** – inline closures cannot capture locals from `InitApp`; enforced by generator.
- **Closure inlining (`-inline`)** – inline simple closures as immediately-invoked function expressions (IIFE), reducing generated function count; identity closures (`func(p T) T { return p }`) collapse to a direct type conversion.
- **Generic‑aware** – supports generic functions and types natively.
- **Observability** – debug logging with runtime‑overridable `Logf`; `-debug` also prints resolved per-package import alias mappings.
- **Actionable errors** – all error messages include source location (`file:line:col`) and `💡 Fix:` suggestions.
- **Unused‑provider policies** – `error` (default), `ignore`, or `drop`.
- **Module nesting** – compose modules hierarchically; duplicate detection built‑in.
- **Named instance injection** – inject multiple instances of the same type by distinguishing them via **parameter names**.
- **ShadowGuard** – generator-level guard that auto-detects and avoids variable-name shadowing in generated code.

---

## Installation

```bash
go get github.com/shanjunmei/dig@latest
go install github.com/shanjunmei/dig/cmd/digen@latest
```
Requires Go 1.25+.

Build with [Mage](https://magefile.org) (optional, auto-injects version info):
```bash
mage build    # build binary with version info from git
mage install  # install to $GOPATH/bin
mage test     # run tests with race detector
```

> **Note:** `cmd/digenv1` in this repository is a legacy single-file generator prototype and is **not** for end users — always use `cmd/digen`.

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

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out dig_gen.go

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
var Logf = log.Printf   // defined in dig_gen.go
func main() { Logf = myLogger.Printf }
```

### 7. Unused Providers
`-unused=error|ignore|drop` (default `error`).

### 8. Package Aliases
`-alias=full|short|obfuscated|numeric` controls generated import aliases. Resolved per-package alias mappings are printed as debug output when `-debug` is enabled.

### 9. Closure Inlining
`-inline` inlines simple provider/invoke closures as IIFEs instead of generating named package-level functions. Identity closures (`func(p T) T { return p }`, `func(p *T) T { return *p }`, `func(p T) *T { return &p }`, and direct type-conversion closures) collapse to a single inline expression.

### 10. Mandatory `//go:build digen` on the DI spec file
Every file that contains a `dig.Build(...)` call **must** carry the `//go:build digen` constraint. digen hard-codes `//go:build !digen` on the generated `dig_gen.go`; without the matching tag on your source, a normal `go build` compiles both files and fails with `InitApp redeclared`. digen now enforces this at generation time — if the tag is missing it prints a clear error with a `💡 Fix:` instead of letting a confusing redeclaration surface later.

### 11. `context.Context` is for `Invoke` only
Providers (constructors registered via `dig.Provide` / `dig.Supply` / `dig.Module`) may **not** declare a `context.Context` parameter. Providers are resolved eagerly inside `InitApp`, before the runtime `context.Context` exists, so the parameter would be undefined in the generated code. Context injection is only valid inside `dig.Invoke(func(ctx context.Context) { ... })`. digen rejects provider-side `context.Context` parameters at generation time with a `💡 Fix:` pointing you to `dig.Invoke` or `dig.Supply`.

### 12. `di.go` holds wiring only: types / constructors / package-level vars must live in a non-`//go:build digen` file or an imported package
The `di.go` file (with `//go:build digen`) may **only** contain the `dig.Build(...)` wiring (Provide / Invoke / Supply / Module calls). Every domain type, constructor, and package-level variable referenced by the wiring must be defined in a file **without that build tag** (e.g. `types.go`) or in an imported package. Reason: the generated `dig_gen.go` carries `//go:build !digen`, so at a normal `go build` (without the digen tag) `di.go` is excluded and its symbols are invisible to the generated code, causing a confusing `undefined: X`. digen now runs a contract pre-check (`checkContractVisibility`) **before** writing any file: if the wiring references a main-package symbol defined inside a digen file, it aborts immediately with a clear error and a `💡 Fix:`, instead of letting the type-checker catch it later.

> **Generation safety net.** After code generation, digen runs a `go/types` type-check on the emitted `dig_gen.go`. Your source is already type-checked when digen loads it, so a type error in the generated file can only be one of two things: (a) a genuine **internal generator bug**, or (b) a **digen-contract violation** that slipped past the pre-check (only possible on an IR-cache hit where the pre-check is skipped). Both abort the write; contract violations get the same `💡 Fix:` guidance as the pre-check, while internal bugs print a **one-click, pre-filled GitHub issue link** (plus a copy-paste template) so the defect is reported upstream — never a silent, uncompilable file.

---

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | `dig_gen.go` | Output filename (ignored in `./...` mode) |
| `-unused` | `error` | Policy for unused providers |
| `-debug` | `false` | Enable debug logging (detailed errors are always shown) |
| `-alias` | `full` | Import alias strategy: `full` / `short` / `obfuscated` / `numeric`. When `-debug` is enabled, resolved per-package alias mappings are also printed. |
| `-inline` | `false` | Inline simple closures as IIFEs; identity closures collapse to a type conversion |
| `-typecheck` | `true` | Type-check generated code after emission to catch internal generator bugs. Disable (`-typecheck=false`) for large `./...` runs where reloading the package graph per file is expensive. |
| `-cache` | `false` | Cache the extracted IR to disk and reuse it for unchanged packages (skips extraction on cache hit). |
| `-cachedir` | `""` | IR cache directory (default: `os.TempDir()/digen-ir-cache`); ignored unless `-cache` is set. |
| `-version` | `false` | Print version information and exit |

---

## CLI Commands

Besides the default generation run (`digen [packages...]`), `digen` provides subcommands for scaffolding, validation, and inspection:

| Command | Description |
|---------|-------------|
| `digen init [path]` | Scaffold a `di.go` containing the `dig.Build` entry point. |
| `digen check [pkgs]` | Validate DI contracts (extraction + unused-provider check) **without** writing any file. |
| `digen graph [pkgs]` | Print the provider dependency graph as a Mermaid flowchart. |
| `digen explain <type> [pkgs]` | Explain how a type/provider is resolved (match by name or return type). |
| `digen completion <shell>` | Print a shell completion script (`bash`, `zsh`, `fish`). |

All flags (`-out`, `-unused`, `-debug`, `-alias`, `-inline`, `-typecheck`, `-cache`, `-cachedir`, `-version`) apply to both the generation run and the `check` / `graph` / `explain` subcommands.

---

## Comparison Matrix

### Architecture & Approach

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| **Approach** | Code generation | Code generation | Runtime reflection |
| Codegen step required | ✅ `digen` | ✅ `wire` CLI | ❌ |
| Zero reflection | ✅ | ✅ | ❌ |
| Zero runtime dependency | ✅ | ✅ | ❌ (requires `fx` + `dig` runtime) |
| Validation timing | Generation time | Generation time | Runtime (`fx.New` / `fx.ValidateApp`) |
| Provider initialization | Eager (at `InitApp` call) | Eager (in generated injector) | Lazy (only if consumed) |
| Binary size impact | Minimal | Minimal | Moderate (`fx` + `dig` + `multierr`) |

### API Design

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| Core API surface | 5 (`Build`/`Provide`/`Supply`/`Invoke`/`Module`) | 7 (`Build`/`NewSet`/`Value`/`InterfaceValue`/`Bind`/`Struct`/`FieldsOf`) | 15+ (`Provide`/`Supply`/`Invoke`/`Module`/`Annotate`/`Annotated`/`Decorate`/`Replace`/`WithLogger`/…) |
| **Direct value injection** | ✅ `dig.Supply` (any expr) | ⚠️ `wire.Value` (no fn calls / channel recv) | ✅ `fx.Supply` (concrete only; interface needs `fx.As`) |
| Built‑in `Invoke` | ✅ | ❌ | ✅ |
| Module definition | `dig.Module(...Option)` | `var Set = wire.NewSet(...)` | `fx.Module("name", ...)` |
| Module nesting | ✅ explicit | ⚠️ flat set composition | ✅ explicit, named |
| Module naming required | ❌ | N/A | ✅ |
| Module scoping (private providers) | ❌ | ❌ | ✅ `fx.Private` |
| Interface binding | via identity closure (e.g. `func(p *Impl) Iface { return p }`, inlined to a conversion) | ✅ explicit `wire.Bind(new(Iface), new(*Impl))` | ✅ `fx.Annotate(NewImpl, fx.As(new(Iface)))` |
| Struct field injection | ❌ | ✅ `wire.Struct` | ❌ (use a constructor) |
| Struct field extraction | ❌ | ✅ `wire.FieldsOf` | ❌ |
| **Multiple instances of same type** | ✅ **Named parameters** | ❌ (must use wrapper types) | ✅ **Named + Value Groups** |
| Value groups (collections of same type) | ❌ | ❌ | ✅ `group:"name"` (with `flatten` / `soft`) |
| Optional dependencies | ❌ | ❌ | ✅ `optional:"true"` |
| Cleanup functions | ❌ | ✅ 2nd return `func()`, ordering guaranteed | ✅ via `OnStop` hooks |
| Lifecycle hooks (OnStart / OnStop) | ❌ | ❌ | ✅ `fx.Lifecycle` |
| Decorators (wrap / replace at runtime) | ❌ | ❌ | ✅ `fx.Decorate` / `fx.Replace` |
| Generic support | ✅ compile‑time (explicit instantiation) | ❌ (must wrap each instantiation) | ⚠️ instantiated generics only; no generic API |
| Closure capture safety | ✅ enforced by generator | N/A (functions only) | N/A |
| API ergonomics | Fx‑style, minimal | Wire‑style, verbose & counter‑intuitive | Fx‑style, minimal |

### Error Handling & Diagnostics

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| Error propagation model | Provider errors `panic` (fail‑fast); Invoke errors returned | Provider `error` return, propagated through injector | `app.Err()`; failed start rolls back `OnStop` hooks |
| Source location in errors | ✅ `file:line:col` on every error | ⚠️ provider / set name only | ⚠️ runtime stack trace |
| Actionable fix suggestions | ✅ `💡 Fix:` on every error | ❌ | ❌ |
| Unused provider policies | 3 modes (`error` / `ignore` / `drop`) | hard error only (no modes) | N/A (lazy; silently skipped) |
| Validation without running | ✅ `digen check` / generation | ✅ (generation = validation) | ✅ `fx.ValidateApp(opts)` |
| Debug logging | ✅ runtime‑overridable `Logf` | ❌ manual | ✅ `fxevent` (Console / Zap / Slog) |
| Dependency graph visualization | ✅ `digen graph` (Mermaid) | ❌ | ✅ `fx.DotGraph` + `fx.VisualizeError` (DOT) |
| Resolution path explanation | ✅ `digen explain <type>` | ❌ (read generated code) | ❌ (runtime errors only) |
| Testing helpers | ❌ | ❌ | ✅ `fxtest` package |

### Runtime & Operations

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| App lifecycle object | ❌ (returns bare `func(ctx) error`) | ❌ (returns generated value) | ✅ `*fx.App` (`Start` / `Stop` / `Done` / `Wait`) |
| Signal handling (SIGINT / SIGTERM) | ❌ (caller's responsibility) | ❌ | ✅ built into `app.Run` |
| Programmatic shutdown | ❌ | ❌ | ✅ `fx.Shutdowner` + `fx.ExitCode` |
| Configurable start / stop timeouts | N/A | N/A | ✅ (default 15s) |

### Project Status

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| Maintenance status | ✅ active | ⚠️ **archived** (bug-fix only) | ✅ active |
| Latest version | v1.0.18 | v0.7.0 (Aug 2025, beta) | v1.24.0 (May 2025) |
| Go version requirement | 1.25+ | standard | 1.21+ (for `slog` logger) |
| Refactoring friendliness | High (static checks + source location) | Low (cryptic errors) | Medium (runtime errors) |

> **Wire specifics**: `wire.Build` requires a dummy `return nil, nil` (or `panic(wire.Build(...))`); `wire.Value` forbids function calls and channel receives (not just constants, but close); `wire.NewSet` composition is flattened during analysis (no scoping / visibility barriers); the project is **archived** as of v0.7.0 — upstream no longer accepts new features, though bug fixes are still accepted; generics are not supported (must write a concrete provider for each instantiation).
>
> **Fx specifics**: richest feature set — full lifecycle (`OnStart`/`OnStop` in dependency order with reverse‑order teardown), decorators (`fx.Decorate`/`fx.Replace`), value groups with `flatten`/`soft` modes, `fx.Private` for module scoping, `fxtest` for testing, `fx.DotGraph` for visualization, and signal‑aware `app.Run` with `fx.Shutdowner`. The cost is runtime reflection (startup latency), runtime panics on wiring errors (mitigated by `fx.ValidateApp` in CI), and a hard dependency on the `fx` + `dig` runtime.
>
> **dig trade‑offs**: deliberately minimal — no lifecycle hooks, no cleanup functions, no decorators, no optional dependencies, no app object / signal handling. `InitApp()` returns a bare `func(context.Context) error`, so graceful shutdown is the caller's responsibility. In exchange: zero runtime overhead, compile‑time safety, source‑located errors with `💡 Fix:` suggestions, native generics, and the smallest API surface of the three.

---

## API Quick Migration Reference

| Operation | dig | Wire | Fx |
|-----------|-----|------|----|
| Constructor | `dig.Provide(NewSvc)` | `wire.NewSet(NewSvc)` | `fx.Provide(NewSvc)` |
| Value injection | `dig.Supply(val)` | `wire.Value(val)` (no fn calls) | `fx.Supply(val)` |
| Startup hook | `dig.Invoke(fn)` | not built‑in | `fx.Invoke(fn)` |
| Module group | `dig.Module(a, b)` | `wire.NewSet(a, b)` | `fx.Module("name", a, b)` |
| Build container | `dig.Build(...)` (returns runnable) | `wire.Build(...)` (dummy marker) | `fx.New(...)` |
| Run | `run := InitApp(); run(ctx)` | call generated function | `app.Run(ctx)` |
| Interface binding | `dig.Provide(func(p *Impl) Iface { return p })` | `wire.Bind(new(Iface), new(*Impl))` | `fx.Annotate(NewImpl, fx.As(new(Iface)))` |
| Multiple instances | named return values + named params | not supported (wrapper types) | `fx.Annotated{Name:"x"}` / `group:"x"` |
| Cleanup / teardown | not supported (caller manages) | provider returns `func()` | `fx.Lifecycle` `OnStop` hook |
| Graceful shutdown | caller handles signals | caller handles signals | `app.Run` (SIGINT/SIGTERM) + `fx.Shutdowner` |

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
