# dig — Compile‑time Dependency Injection for Go

[中文文档](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **📢 Version Note**
> - **v1.0.4** – last release with `*dig.App` struct
> - **v1.0.5** – `InitApp()` returns `func(context.Context) error`, with zero runtime dependency
> See [Upgrading from v1.0.4](#upgrading-from-v104) for migration instructions.

**dig** is a code‑generation based dependency injection container for Go.  
It resolves all dependencies at **compile time** and generates plain Go source code – **no reflection**, no runtime magic, just static, native Go.

---

## Why dig?

- **Zero reflection** – startup performance equals hand‑written initialization code.
- **Static safety** – missing or circular dependencies are caught during `go generate`, not at runtime.
- **Minimal API** – only 4 core functions: `Build`, `Provide`, `Supply`, `Invoke`.
- **Built‑in `Supply`** – inject package‑level global variables directly, no verbose wrappers.
- **Wrapper types** – define lightweight aliases to resolve primitive type injection conflicts.
- **Built‑in `Invoke`** – all startup/registration logic runs after all providers are ready.
- **Observability** – optional debug logging with `before/after` markers; `Logf` can be overridden at runtime.
- **Unused‑provider policies** – choose `error` (default), `ignore`, or `drop`.
- **Zero runtime dependency** – generated code does not import the `dig` package at runtime.
- **Small binary size** – no embedded reflection or framework runtime.
- **Low learning curve** – just 4 APIs and a few clear constraints.

---

## Installation

```bash
go get github.com/shanjunmei/dig@v1.0.5
go install github.com/shanjunmei/dig/cmd/digen@latest
```

Requires Go 1.21+.

---

## 1. Basic Usage

The simplest way to use **dig** involves two files.

### 1.1 Container Definition (`di.go`)

This file is the **only** file you need to write for the DI wiring. It uses the `//go:build digen` build tag so it is only parsed during code generation.

```go
//go:build digen
package main

import (
    "context"
    "fmt"
    "github.com/shanjunmei/dig"
)

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out di_gen.go

func InitApp() func(context.Context) error {
    return dig.Build(
        // 1) Ordinary function constructors
        dig.Provide(NewConfig),
        dig.Provide(NewDB),

        // 2) Provide a value directly (useful for constants or globals)
        dig.Supply(DefaultTimeout),

        // 3) Provide using an inline closure – acceptable as long as
        //    it only uses constant literals or package-level variables.
        dig.Provide(func(timeout Timeout) *Server {
            return NewServer(timeout)
        }),

        // 4) Invoke runs after all providers are ready.
        //    It can return an error which will be propagated to the caller.
        dig.Invoke(func(srv *Server) error {
            return srv.Run()
        }),
    )
}
```

### 1.2 Business Logic (`main.go`)

This file contains all your types, constructors, and the `main()` function. It has **no build tag**.

```go
package main

import (
    "context"
    "fmt"
    "time"
)

// ---------- Config ----------
type Config struct {
    Addr string
}

func NewConfig() (*Config, error) {
    return &Config{Addr: ":8080"}, nil
}

// ---------- DB ----------
type DB struct {
    cfg *Config
}

func NewDB(cfg *Config) (*DB, error) {
    if cfg == nil {
        return nil, fmt.Errorf("config cannot be nil")
    }
    return &DB{cfg: cfg}, nil
}

func (db *DB) Ping() error {
    fmt.Println("DB ping OK")
    return nil
}

// ---------- Timeout (wrapper type) ----------
type Timeout time.Duration

var DefaultTimeout = Timeout(5 * time.Second)

// ---------- Server ----------
type Server struct {
    db      *DB
    timeout Timeout
}

func NewServer(timeout Timeout) *Server {
    return &Server{timeout: timeout}
}

func (s *Server) Run() error {
    fmt.Printf("Server running with timeout %v\n", s.timeout)
    return nil
}

// ---------- Main ----------
func main() {
    run := InitApp()
    if err := run(context.Background()); err != nil {
        fmt.Printf("app failed: %v\n", err)
    }
}
```

### 1.3 Generate and Run

There are two ways to invoke the generator:

```bash
# Generate only the current package (default)
digen

# Generate for all sub‑packages recursively
digen ./...
```

Or via `go generate` (if your `di.go` contains a `//go:generate` directive):

```bash
go generate ./...
```

After generation, build and run your application:

```bash
go run .
```

The generator outputs `dig_gen.go` in each package directory that contains a `dig.Build` call. Each generated file is self‑contained with no runtime dependency on the `dig` package.

---

## 2. Advanced Usage

### 2.1 Supply – Injecting Existing Values

`Supply` injects a value directly without a constructor function. It's perfect for constants, package‑level variables, or values computed at runtime.

```go
dig.Supply(globalDBConn)
dig.Supply(apiKey)
dig.Supply(DefaultTimeout)
```

### 2.2 Wrapper Types to Resolve Primitive Conflicts

If you need multiple `bool`, `string`, or `time.Duration` values, wrap them in distinct types to avoid duplicate provider errors.

```go
// main.go
package main

type UseMySQL bool
type UseRedis bool
type QueryTimeout time.Duration
type DialTimeout time.Duration
```

In `di.go`:
```go
dig.Provide(func() UseMySQL { return UseMySQL(true) }),
dig.Provide(func() UseRedis { return UseRedis(false) }),
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) }),
dig.Provide(func() DialTimeout { return DialTimeout(2 * time.Second) }),
```

The generator sees each wrapper type as unique, so there is no conflict.

### 2.3 Closure Constraint (Critical)

When you write an inline anonymous function inside `dig.Provide` or `dig.Invoke`, you **must not** capture variables from the outer scope of `InitApp()`.

✅ **Correct** – uses only constant literals or package‑level variables:
```go
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) })
dig.Provide(func() Timeout { return DefaultTimeout }) // DefaultTimeout is package-level
```

❌ **Wrong** – captures a local variable `t`:
```go
t := 5 * time.Second
dig.Provide(func() QueryTimeout { return QueryTimeout(t) })
// generator error: cannot capture local variable "t"
```

**Why?** The generator extracts the closure body and lifts it into a top‑level function in `di_gen.go`. This new function is defined at **package level** – it does **not** have access to `InitApp()`'s stack frame. Only package‑level symbols (global variables, constants, functions, types) and literal values are accessible.

**What about `InitApp` parameters?** They are also local to `InitApp` and cannot be captured either. However, `digen` automatically registers all `InitApp` parameters as `Supply` providers, so you don't need to capture them – you can simply request them in `Provide`/`Invoke` closures.

### 2.4 External Parameters (InitApp Arguments)

You can pass external dependencies into `InitApp` as parameters. The generator automatically registers each parameter as a `Supply` provider, making them available for injection.

```go
// di.go
//go:build digen
func InitApp(cfg *Config, logger *Logger) func(context.Context) error {
    return dig.Build(
        dig.Provide(NewDB),          // can depend on *Config (from parameter)
        dig.Invoke(func(db *DB) error { // can also depend on *Config or *Logger
            logger.Info("db initialized")
            return nil
        }),
    )
}
```

**How it works**: The generator adds a `Supply` for each parameter of `InitApp`, so you can directly use those types in your providers and invokes. They are treated exactly like explicit `dig.Supply` calls.

**Key distinction**: 
- **InitApp parameters** are injected automatically as supplies.
- **Local variables** defined inside `InitApp` cannot be captured by closures.

If you need to provide a value that is computed at runtime inside `InitApp`, define it as a package‑level variable or function, or use `dig.Supply` with a package‑level variable.

### 2.5 Generic Support

`digen` fully supports generic functions and types. You can use them directly in `Provide` and `Invoke`.

**Generic types**:
```go
type Store[T any] struct { items []T }
func NewStore[T any]() *Store[T] { return &Store[T]{} }
```

In `di.go`:
```go
dig.Provide(NewStore[int])          // concrete instantiation
dig.Provide(NewStore[string])       // another concrete instantiation
dig.Invoke(func(s *Store[int]) error { ... })
```

**Generic functions**:
```go
func Process[T any](s *Store[T]) error { ... }
```

```go
dig.Invoke(Process[int])            // instantiate generic function
dig.Invoke(Process[string])
```

**Generic closures**:
```go
dig.Provide(func() *Store[bool] { return NewStore[bool]() })
```

The generator handles generic instantiations correctly and includes type arguments in the generated function calls.

### 2.6 Conditional Logic

Because dig is a **code generator**, not a runtime framework, conditional logic works correctly **inside** `Provide`/`Invoke` closures, but not inside `Module()` functions.

✅ **Inside closures – works at runtime**:
```go
dig.Provide(func(useMySQL UseMySQL) Store {
    if useMySQL {
        return NewMySQLStore()
    }
    return NewRedisStore()
})
```

❌ **Inside `Module()` – does NOT work as expected** (all branches are parsed and registered):
```go
func Module() dig.Option {
    if enableCache {
        return dig.Module(dig.Provide(NewCache))
    }
    return dig.Module(dig.Provide(NewNoop))
}
// Both NewCache and NewNoop will be registered
```

**Use build tags for compile‑time selection**:
```go
// module_cache.go
//go:build enable_cache
func Module() dig.Option { return dig.Module(dig.Provide(NewCache)) }

// module_noop.go
//go:build !enable_cache
func Module() dig.Option { return dig.Module(dig.Provide(NewNoop)) }
```

### 2.7 Observability (Debug Logging & Custom Logging)

Enable debug logging with the `-debug` flag when running `digen`. The generated code will insert `Logf` calls around each provider and invoker.

Override `Logf` at runtime in your `main.go`:

```go
var Logf = log.Printf // declared in dig_gen.go

func main() {
    Logf = myLogger.Printf // custom logger
    // ...
}
```

### 2.8 Unused Provider Policies

- **`error`** (default) – generation fails if unused providers exist.
- **`ignore`** – keep unused providers with `_ = fn()`.
- **`drop`** – remove unused providers entirely.

```bash
digen -unused=drop -out di_gen.go
```

### 2.9 Package Alias Strategies

- **`full`** (default) – `addr_handler`, `user_handler`
- **`short`** – `handler`, `handler2`
- **`obfuscated`** – `a`, `b`, `c1`

### 2.10 Module‑Style Code Organisation with `dig.Module`

For larger projects, each module defines its own `Module()` function that returns a `dig.Option`. Modules can be **nested** – a module can include other modules, allowing hierarchical organisation.

**Project structure**:

```
myapp/
|-- di.go                         # top-level composition
|-- main.go
|-- internal/
|   |-- db/
|   |   `-- module.go             # db.Module()
|   |-- server/
|   |   `-- module.go             # server.Module() – may include db.Module()
|   |-- logger/
|   |   `-- module.go             # logger.Module()
|   `-- monitoring/
|       `-- module.go             # monitoring.Module() – may include logger.Module()
`-- pkg/
    `-- common/
        `-- timeout.go
```

**Example: nested modules**

**`internal/db/module.go`**:
```go
package db

import "github.com/shanjunmei/dig"

func Module() dig.Option {
    return dig.Module(
        dig.Provide(NewConfig),
        dig.Provide(NewConnection),
        dig.Invoke(func(db *DB) error { return db.Ping() }),
    )
}
```

**`internal/logger/module.go`**:
```go
package logger

import "github.com/shanjunmei/dig"

func Module() dig.Option {
    return dig.Module(
        dig.Provide(New),
        dig.Invoke(Init),
    )
}
```

**`internal/monitoring/module.go`** (nested module):
```go
package monitoring

import (
    "myapp/internal/logger"
    "github.com/shanjunmei/dig"
)

func Module() dig.Option {
    return dig.Module(
        logger.Module(),
        dig.Provide(NewMetricsCollector),
        dig.Provide(NewHealthChecker),
        dig.Invoke(StartMetricsServer),
    )
}
```

**`internal/server/module.go`** (nested module):
```go
package server

import (
    "myapp/internal/db"
    "myapp/internal/monitoring"
    "github.com/shanjunmei/dig"
)

func Module() dig.Option {
    return dig.Module(
        db.Module(),
        monitoring.Module(),
        dig.Provide(New),
        dig.Provide(NewRouter),
        dig.Invoke(RegisterRoutes),
    )
}
```

**`di.go`** – top‑level composition:
```go
//go:build digen
package main

import (
    "myapp/internal/server"
    "myapp/internal/logger"
    "myapp/pkg/common"
    "github.com/shanjunmei/dig"
)

func InitApp() func(context.Context) error {
    return dig.Build(
        server.Module(),
        dig.Supply(common.DefaultTimeout),
        dig.Provide(NewGlobalService),
        logger.Module(), // safe if not already included transitively
        dig.Invoke(StartGlobalWorker),
    )
}
```

**Nesting benefits**:
- Encapsulates complex dependency hierarchies within modules.
- Modules can be reused and composed independently.
- The top‑level `di.go` stays clean even as the project grows.

**Important**: Do not include the same module twice (directly or transitively) – the generator will report a duplicate provider error. Design your module hierarchy so each module is included only once.

---

## 3. Complete Example

The [`example/`](./example) directory contains a comprehensive demonstration covering all features:

- Cross‑package dependencies
- Generic types and functions
- Same‑name modules from different paths (`user/repository` and `role/repository`)
- Module nesting
- External parameters (`InitApp` with arguments)
- Supply with type conversions
- Closure usage
- Debug logging
- Build tags
- Package alias strategies

To run the example:

```bash
cd example
go generate ./...
go run .
```

Refer to the example code for a full-featured real-world style dependency injection setup.

---

## CLI Flags (Full Reference)

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | `di_gen.go` | Output filename |
| `-unused` | `error` | Behaviour for unused providers: `error`, `ignore`, `drop` |
| `-debug` | `false` | Enable debug logs in generated code (uses `Logf`) |
| `-alias` | `full` | Package alias strategy: `short`, `full`, or `obfuscated` |

> **Note:** When `digen` is invoked with `./...` (multi‑package mode), the `-out` flag is ignored and `dig_gen.go` is used in each package directory.

---

## Upgrading from v1.0.4

In v1.0.4 and earlier, `InitApp()` returned `*dig.App` with a `Run` method:

```go
app := InitApp()
if err := app.Run(context.Background()); err != nil {
    log.Fatal(err)
}
```

In **v1.0.5**, `InitApp()` returns `func(context.Context) error` directly:

```go
run := InitApp()
if err := run(context.Background()); err != nil {
    log.Fatal(err)
}
```

**Migration steps:**

1. Change `app.Run(ctx)` to `run(ctx)` where `run := InitApp()`
2. Remove any references to the `dig.App` type in your code
3. Update your `di.go` signature from `func InitApp() *dig.App` to `func InitApp() func(context.Context) error`
4. Run `go generate` to regenerate `di_gen.go`
5. Run `go mod tidy` to update dependencies

**Why this change?** – Starting from v1.0.5, the generated code no longer imports the `dig` package at runtime. This eliminates the runtime dependency entirely, resulting in smaller binaries and zero reflection overhead.

---

## Comparison with Other DI Tools

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| **Approach** | Code generation (compile‑time) | Code generation (compile‑time) | Runtime reflection |
| **Code generation workflow** | ✅ `digen` CLI | ✅ `wire` CLI | N/A |
| **Zero runtime reflection** | ✅ | ✅ | N/A |
| **Zero runtime dependency** | ✅ | ✅ | N/A (requires `fx` runtime) |
| **Dependency validation** | At generation time | At generation time | At runtime |
| **Dedicated `Supply` API** | ✅ (`dig.Supply`) | ✅ (`wire.Value`/`wire.InterfaceValue`) | ✅ (`fx.Supply`) |
| **Direct value injection cost** | Zero (compile‑time) | Zero (compile‑time) | Runtime reflection overhead |
| **Closure safety enforcement** | ✅ (capture check) | ⚠️ (no check) | N/A |
| **Wrapper type support** | ✅ | ⚠️ (manual) | N/A |
| **Built‑in `Invoke`** | ✅ | N/A | ✅ (lifecycle hooks) |
| **Module composition** | ✅ (`dig.Module`, with nesting) | ✅ (`wire.NewSet`, with nesting) | ✅ (`fx.Module`) |
| **Generic support** | ✅ (compile‑time, full) | ✅ (requires explicit instantiation) | ✅ (runtime, via reflection) |
| **Unused provider policies** | 3 modes | only `drop` | N/A |
| **Built‑in debug logging** | ✅ (with runtime override) | ⚠️ (manual) | ✅ (tracing) |
| **External dependencies** | none (std only) | none | many |
| **Generated code size** | Compact | Verbose | N/A |
| **Generation performance** | Fast (AST rewrite) | Slower (full type‑checking) | N/A |
| **Learning curve** | Low | Medium | High |

---

## License

MIT – see [LICENSE](LICENSE) for details.
