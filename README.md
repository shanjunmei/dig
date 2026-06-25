# dig — Compile&#8209;time Dependency Injection for Go

[&#20013;&#25991;&#25991;&#26723;](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**dig** is a code&#8209;generation based dependency injection container for Go.  
It resolves all dependencies at **compile time** and generates plain Go source code – **no reflection**, no runtime magic, just static, native Go.

---

## Why dig?

- **Zero reflection** – startup performance equals hand&#8209;written initialization code.
- **Static safety** – missing or circular dependencies are caught during `go generate`, not at runtime.
- **Minimal API** – only 4 core functions: `Build`, `Provide`, `Supply`, `Invoke`.
- **Safe closures** – inline `Provide` functions are restricted to constant literals only; capturing outer local variables is forbidden to avoid undefined symbols in generated code.
- **Built&#8209;in `Supply`** – inject package&#8209;level global variables directly, no verbose wrappers.
- **Wrapper types** – define lightweight aliases to resolve primitive type injection conflicts.
- **Built&#8209;in `Invoke`** – all startup/registration logic runs after all providers are ready.
- **Observability** – optional debug logging with `before/after` markers; `Logf` can be overridden at runtime.
- **Unused&#8209;provider policies** – choose `error` (default), `ignore`, or `drop`.
- **No external dependencies** – core library uses only the Go standard library.
- **Small binary size** – no embedded reflection or framework runtime.
- **Low learning curve** – just 4 APIs and a few clear constraints.

---

## Installation

```bash
go get github.com/shanjunmei/dig
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

func InitApp() *dig.App {
    return dig.Build(
        // 1) Ordinary function constructors
        dig.Provide(NewConfig),
        dig.Provide(NewDB),

        // 2) Provide a value directly (useful for constants or globals)
        dig.Supply(DefaultTimeout),

        // 3) Provide using an inline closure – acceptable as long as
        //    it only uses constant literals or supplied globals.
        dig.Provide(func(timeout Timeout) *Server {
            // The closure body can be more complex, but must not capture
            // any local variable from InitApp().
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
    // In real code, you might read from env/file.
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

var DefaultTimeout = Timeout(5 * time.Second) // used by dig.Supply

// ---------- Server ----------
type Server struct {
    db      *DB
    timeout Timeout
}

// NewServer is called by the closure above
func NewServer(timeout Timeout) *Server {
    // in reality we'd use timeout to set timeouts
    return &Server{timeout: timeout}
}

func (s *Server) Run() error {
    fmt.Printf("Server running with timeout %v\n", s.timeout)
    return nil
}

// ---------- Main ----------
func main() {
    app := InitApp() // generated in di_gen.go
    if err := app.Run(context.Background()); err != nil {
        fmt.Printf("app failed: %v\n", err)
    }
}
```

### 1.3 Generate and Run

```bash
go generate ./...
go run .
```

The generator resolves dependencies and produces `di_gen.go`.

---

## 2. Advanced Usage

### 2.1 Supply – Injecting Existing Values

You've already seen `Supply` in the basic example – it injects a package&#8209;level variable. You can use it for any global, constant, or runtime&#8209;computed value.

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

&#9989; **Correct** – uses only constant literals:
```go
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) })
```

&#10060; **Wrong** – captures a local variable `t`:
```go
t := 5 * time.Second
dig.Provide(func() QueryTimeout { return QueryTimeout(t) })
// generator error: can't capture local variable 't'
```

**Why?**  
The generator extracts the closure body and lifts it into a top&#8209;level function in `di_gen.go`. This new function is defined at **package level** – it does **not** have access to `InitApp()`'s stack frame. If the closure captures a local variable from `InitApp()`, that variable does not exist in the package scope, causing an "undefined symbol" compile error.

Even if the captured value is a constant, it is still bound to `InitApp`'s scope. Only **constant literals** and **package&#8209;level variables** (which you can supply via `dig.Supply`) are allowed, because they are resolvable at package level.

If you need a value that is computed at runtime, define it as a package&#8209;level variable and `dig.Supply` it.

### 2.4 Observability (Debug Logging & Custom Logging)

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

    app := InitApp()
    if err := app.Run(context.Background()); err != nil {
        customLogger.Fatalf("app failed: %v", err)
    }
}
```

**No external dependency** – `Logf` uses standard `log` by default.

### 2.5 Unused Provider Policies

- **`error`** (default) – generation fails if unused providers exist.
- **`ignore`** – keep unused providers with `_ = fn()`.
- **`drop`** – remove unused providers entirely.

```bash
digen -unused=drop -out di_gen.go
```

### 2.6 Package Alias Strategies

- **`full`** (default) – `addr_handler`, `user_handler`
- **`short`** – `handler`, `handler2`
- **`obfuscated`** – `a`, `b`, `c1`

### 2.7 Module&#8209;Style Code Organisation with `dig.Module`

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
        // Include logger module as a dependency
        logger.Module(),

        // Monitoring's own providers
        dig.Provide(NewMetricsCollector),
        dig.Provide(NewHealthChecker),

        // Monitoring's own invokes
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
        // Include db and monitoring modules
        db.Module(),
        monitoring.Module(),

        // Server's own providers
        dig.Provide(New),
        dig.Provide(NewRouter),

        // Server's invokes
        dig.Invoke(RegisterRoutes),
    )
}
```

**`di.go`** – top&#8209;level composition (mixing modules with plain providers):
```go
//go:build digen
package main

import (
    "myapp/internal/server"
    "myapp/internal/logger"
    "myapp/pkg/common"
    "github.com/shanjunmei/dig"
)

func InitApp() *dig.App {
    return dig.Build(
        // Include the server module – it already includes db, monitoring, etc.
        server.Module(),

        // Top&#8209;level providers and supplies can be mixed freely
        dig.Supply(common.DefaultTimeout),
        dig.Provide(NewGlobalService),

        // Another module can be included here, but be careful:
        // if it's already included transitively, avoid duplicate inclusion.
        // The generator will fail if the same provider is registered twice.
        // In this example, logger.Module() is not included by server.Module(),
        // so it's safe to add it here.
        logger.Module(),

        // Top&#8209;level invoke
        dig.Invoke(StartGlobalWorker),
    )
}
```

**Nesting benefits**:
- Encapsulates complex dependency hierarchies within modules.
- Modules can be reused and composed independently.
- The top&#8209;level `di.go` stays clean even as the project grows.

**Important**: Do not include the same module twice (directly or transitively) – the generator will report a duplicate provider error. Design your module hierarchy so each module is included only once, or use a pattern where the top&#8209;level `di.go` is the single source of truth for module composition.

---

## CLI Flags (Full Reference)

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | `di_gen.go` | Output filename |
| `-unused` | `error` | Behaviour for unused providers: `error`, `ignore`, `drop` |
| `-debug` | `false` | Enable debug logs in generated code (uses `Logf`) |
| `-alias` | `full` | Package alias strategy: `short`, `full`, or `obfuscated` |

---

## Comparison with Other DI Tools

| Feature | dig | Google Wire | Uber dig / FX |
|---------|-----|-------------|---------------|
| **Approach** | Code generation (compile&#8209;time) | Code generation (compile&#8209;time) | Runtime reflection (no generation) |
| **Code generation workflow** | &#9989; `digen` CLI | &#9989; `wire` CLI | &#10060; Not applicable (reflection&#8209;based) |
| **Zero runtime reflection** | &#9989; | &#9989; | &#10060; |
| **Dependency validation** | At generation time | At generation time | At runtime |
| **Dedicated `Supply` API** | &#9989; | &#10060; | &#10060; |
| **Closure safety enforcement** | &#9989; (capture check) | &#9888;&#65039; (no check) | N/A |
| **Wrapper type support** | &#9989; | &#9888;&#65039; (manual) | &#10060; |
| **Built&#8209;in `Invoke`** | &#9989; | &#10060; | &#9989; (lifecycle hooks) |
| **`dig.Module` composition** | &#9989; (with nesting) | &#10060; | &#9989; (fx.Module) |
| **Unused provider policies** | 3 modes | only `drop` | N/A |
| **Built&#8209;in debug logging** | &#9989; (with runtime override) | &#9888;&#65039; (manual) | &#9989; (tracing) |
| **External dependencies** | none (std only) | none | many |
| **Generated code size** | Compact | Verbose | N/A |
| **Generation performance** | Fast (AST rewrite) | Slower (full type&#8209;checking) | N/A |
| **Learning curve** | Low | Medium | High |

---

## License

MIT – see [LICENSE](LICENSE) for details.
