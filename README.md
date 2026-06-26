# dig — Compile‑time Dependency Injection for Go

[中文文档](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **📢 Version Note**
> - **v1.0.4** – last release with `*dig.App` struct
> - **v2.0.0** – `InitApp()` returns `func(context.Context) error`
> See [Upgrading from v1.x](#upgrading-from-v1x) for migration instructions.

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
go get github.com/shanjunmei/dig@v2.0.0
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
        //    it only uses constant literals or supplied globals.
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

You've already seen `Supply` in the basic example – it injects a package‑level variable. You can use it for any global, constant, or runtime‑computed value.

```go
dig.Supply(globalDBConn)
dig.Supply(apiKey)
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

When you write an inline anonymous function inside `dig.Provide`, you **must not** capture variables from the outer scope of `InitApp()`.

✅ **Correct** – uses only constant literals:
```go
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) })
```

❌ **Wrong** – captures a local variable `t`:
```go
t := 5 * time.Second
dig.Provide(func() QueryTimeout { return QueryTimeout(t) })
// generator error: cannot capture local variable "t"
```

**Why?**  
The generator extracts the closure body and lifts it into a top‑level function in `di_gen.go`. This new function is defined at **package level** – it does **not** have access to `InitApp()`'s stack frame. If the closure captures a local variable from `InitApp()`, that variable does not exist in the package scope, causing an "undefined symbol" compile error.

**Important**: Even if the captured value is a constant, it is still bound to `InitApp`'s scope. Only **constant literals** (like `true`, `3`, `"hello"`) and **package‑level variables/constants** are allowed, because they are resolvable at package level.

**If you need a value that is computed at runtime, define it as a package‑level variable and use `dig.Supply` to inject it.**

### 2.4 Where Conditional Logic Works (and Where It Doesn't)

Because dig is a **code generator**, not a runtime framework, it performs **static analysis** on your `di.go` and module files. This means the generator reads your code as text, but **does not execute it**. Therefore, conditional logic works differently depending on where you place it.

#### ✅ Conditionals inside `Provide`/`Invoke` closures (works)

The body of a closure passed to `dig.Provide` or `dig.Invoke` is copied as-is into the generated code. All conditional logic inside it will execute **at runtime** as expected.

**Example (classic conditional injection):**
```go
dig.Provide(func(t QueryTimeout, useMySQL UseMySQL, cache EnableCache) Store {
    if useMySQL {
        return NewMySQLStore(t)
    }
    return NewRedisStore(cache)
})
```

The generator copies the entire closure body into a generated function like `__p_xxx`. At runtime, `useMySQL` determines which store is created.

**This is the recommended way to handle conditional logic.**

#### ❌ Conditionals inside `Module()` function body (does NOT work)

The `Module()` function is parsed statically. The generator **does not execute** any `if` statements, loops, or branches inside it. All `dig.Provide`, `dig.Invoke`, and `dig.Supply` calls are extracted regardless of which branch they appear in.

**Example of what does NOT work as expected:**
```go
func Module() dig.Option {
    if enableCache {
        return dig.Module(
            dig.Provide(NewCache),
            dig.Invoke(StartCache),
        )
    }
    return dig.Module(
        dig.Provide(NewNoop),
        dig.Invoke(StartNoop),
    )
}
```

The generator will parse **both branches** and register **all** providers (`NewCache`, `NewNoop`, `StartCache`, `StartNoop`) into the dependency graph, regardless of `enableCache`. The condition is never evaluated during generation.

**To achieve conditional module inclusion**, use **build tags** instead:

```go
// module_cache.go
//go:build enable_cache
package mod
func Module() dig.Option { return dig.Module(dig.Provide(NewCache), dig.Invoke(StartCache)) }

// module_noop.go
//go:build !enable_cache
package mod
func Module() dig.Option { return dig.Module(dig.Provide(NewNoop), dig.Invoke(StartNoop)) }
```

Then in `di.go`:
```go
func InitApp() func(context.Context) error {
    return dig.Build(
        mod.Module(), // build tags decide which file is compiled
    )
}
```

#### ✅ Conditionals inside `Invoke` closures (works)

Same as `Provide` closures — the entire body is copied and runs at runtime.

```go
dig.Invoke(func(config Config) {
    if config.Debug {
        log.Println("debug mode enabled")
    }
})
```

#### ❌ `dig.Module` with IIFE (not recommended)

You may attempt to use an immediately-invoked function expression (IIFE) inside `dig.Module`:

```go
dig.Module(
    func() dig.Option {
        if someCondition {
            return dig.Provide(NewCache)
        }
        return dig.Provide(NewNoop)
    }(),
)
```

This does NOT work because `someCondition` cannot be evaluated at generation time. The generator will parse both branches and register both providers. Avoid this pattern.

#### Summary Table

| Location | Conditional Logic Works? | Why |
|----------|--------------------------|-----|
| Inside `Provide`/`Invoke` closure body | ✅ Yes | Body is copied and runs at runtime |
| Inside `Module()` function body | ❌ No | Generator does not execute control flow |
| Inside `dig.Module` arguments via IIFE | ❌ No | Condition cannot be evaluated at generation time |
| Using build tags | ✅ Yes | Compile-time selection controlled by Go |

**Rule of thumb:**
- Put runtime decisions **inside** `Provide`/`Invoke` closures.
- Use **build tags** for compile-time module selection.
- Keep `Module()` functions **pure** — only `dig.Module` calls and `return`.

### 2.5 Observability (Debug Logging & Custom Logging)

**Enable debug logging** with the `-debug` flag.

Run the generator directly:
```bash
digen -debug -out di_gen.go
```

Or add `-debug` to the `//go:generate` directive in `di.go`:
```go
//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -debug -out di_gen.go
```

When enabled, the generated code will insert `Logf` calls like this:

```go
Logf("[PROVIDE] before: %s\n", "main.NewConfig")
v0, err := NewConfig()
if err != nil {
    Logf("[PROVIDE] failed: %s: %v\n", "main.NewConfig", err)
    panic(err)
}
Logf("[PROVIDE] after: %s\n", "main.NewConfig")
```

You'll see runtime output such as:

```
[PROVIDE] before: main.NewConfig
[PROVIDE] after:  main.NewConfig
[PROVIDE] before: main.NewDB
[PROVIDE] after:  main.NewDB
[INVOKE]  before: main.(*Server).Run
[INVOKE]  after:  main.(*Server).Run
```

**Override `Logf` at runtime** – The generated file declares:

```go
var Logf = log.Printf
```

You can override this in your `main.go` before calling `InitApp()`:

```go
package main

import (
    "log"
    "os"
)

func main() {
    customLogger := log.New(os.Stdout, "[MYAPP] ", log.LstdFlags|log.Lshortfile)
    Logf = customLogger.Printf

    run := InitApp()
    if err := run(context.Background()); err != nil {
        customLogger.Fatalf("app failed: %v", err)
    }
}
```

**No external dependency** – `Logf` uses standard `log` by default.

### 2.6 Unused Provider Policies

- **`error`** (default) – generation fails if unused providers exist.
- **`ignore`** – keep unused providers with `_ = fn()`.
- **`drop`** – remove unused providers entirely.

```bash
digen -unused=drop -out di_gen.go
```

### 2.7 Package Alias Strategies

- **`full`** (default) – `addr_handler`, `user_handler`
- **`short`** – `handler`, `handler2`
- **`obfuscated`** – `a`, `b`, `c1`

### 2.8 Module‑Style Code Organisation with `dig.Module`

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

**`di.go`** – top‑level composition (mixing modules with plain providers):
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

## CLI Flags (Full Reference)

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | `di_gen.go` | Output filename |
| `-unused` | `error` | Behaviour for unused providers: `error`, `ignore`, `drop` |
| `-debug` | `false` | Enable debug logs in generated code (uses `Logf`) |
| `-alias` | `full` | Package alias strategy: `short`, `full`, or `obfuscated` |

> **Note:** When `digen` is invoked with `./...` (multi‑package mode), the `-out` flag is ignored and `dig_gen.go` is used in each package directory.

---

## Upgrading from v1.x

In v1.x (up to v1.0.4), `InitApp()` returned `*dig.App` with a `Run` method:

```go
app := InitApp()
if err := app.Run(context.Background()); err != nil {
    log.Fatal(err)
}
```

In **v2.0.0**, `InitApp()` returns `func(context.Context) error` directly:

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

**Why this change?** – Starting from v2.0.0, the generated code no longer imports the `dig` package at runtime. This eliminates the runtime dependency entirely, resulting in smaller binaries and zero reflection overhead.

---

## Comparison with Other DI Tools

| Feature | dig | Google Wire | Uber dig / FX |
|---------|-----|-------------|---------------|
| **Approach** | Code generation (compile‑time) | Code generation (compile‑time) | Runtime reflection (no generation) |
| **Code generation workflow** | ✅ `digen` CLI | ✅ `wire` CLI | ❌ Not applicable |
| **Zero runtime reflection** | ✅ | ✅ | ❌ |
| **Zero runtime dependency** | ✅ | ✅ | ❌ |
| **Dependency validation** | At generation time | At generation time | At runtime |
| **Dedicated `Supply` API** | ✅ | ❌ | ❌ |
| **Closure safety enforcement** | ✅ (capture check) | ⚠️ (no check) | N/A |
| **Wrapper type support** | ✅ | ⚠️ (manual) | ❌ |
| **Built‑in `Invoke`** | ✅ | ❌ | ✅ (lifecycle hooks) |
| **`dig.Module` composition** | ✅ (with nesting) | ❌ | ✅ (fx.Module) |
| **Unused provider policies** | 3 modes | only `drop` | N/A |
| **Built‑in debug logging** | ✅ (with runtime override) | ⚠️ (manual) | ✅ (tracing) |
| **External dependencies** | none (std only) | none | many |
| **Generated code size** | Compact | Verbose | N/A |
| **Generation performance** | Fast (AST rewrite) | Slower (full type‑checking) | N/A |
| **Learning curve** | Low | Medium | High |

---

## License

MIT – see [LICENSE](LICENSE) for details.
