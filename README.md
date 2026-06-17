# dig — Code-Generation Zero-Reflection DI Container for Go
[&#20013;&#25991;](README_zh.md) | **English**

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache2.0-yellow.svg)](https://opensource.org/licenses/Apache-2.0)

**dig** is a compile-time code generation dependency injection container for Go.
It eliminates runtime reflection overhead entirely, validates the dependency graph at code generation stage, and provides only four minimal core APIs: `Provide`, `Invoke`, `Supply`, `Module`, following idiomatic Go patterns.

Core design philosophy:
> Static safety at generation time, zero overhead at runtime, no magic, stay native Go.

## Core Advantages
1. **Zero runtime reflection**
All dependency resolution logic is generated as plain Go code via `digen`. There is no runtime type inspection, and startup performance matches handwritten initialization logic.
2. **Full static validation before deployment**
Circular dependencies, missing providers, unexported cross-package types, duplicate registrations, unused constructors will throw clear errors when running `go generate`, avoiding online panics.
3. **Native closure support**
Closure constructors and closure startup callbacks are fully supported. The generator automatically captures outer free variables and generates complete parameter injection logic.
4. **Flexible module composition**
Group providers and invokers by business domain with `Module`. Multiple modules can be combined seamlessly without lifecycle coupling.
5. **Direct supply for existing instances**
Use `Supply` to inject pre-initialized objects (database connections, global configs, clients) without wrapping values into constructors.
6. **Lightweight dependency footprint**
Business code only relies on the Go standard library. The CLI generator only depends on official `golang.org/x/tools` for AST analysis, no heavy third-party dependencies.
7. **Configurable unused constructor policy**
Three strategies for handling unused constructors:
- `error`: Fail code generation, force cleanup of redundant constructors
- `ignore`: Suppress warnings silently
- `drop`: Remove unused code from the generated file

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

## Quick Start
### 1. Write business code & app init entry
```go
// main.go
package main

import (
	"context"
	"github.com/shanjunmei/dig"
)

// Config global application configuration
type Config struct {
	Addr string
}

// NewConfig config constructor
func NewConfig() *Config {
	return &Config{Addr: "0.0.0.0:8080"}
}

// UserService business service depends on Config
type UserService struct {
	cfg *Config
}

func NewUserService(cfg *Config) *UserService {
	return &UserService{cfg: cfg}
}

// StartService startup callback, executed after all dependencies built
func StartService(svc *UserService) error {
	println("server listen on", svc.cfg.Addr)
	return nil
}

// InitApp global root init entry, only one Build() per package
func InitApp() *dig.App {
	return dig.Build(
		dig.Provide(NewConfig),
		dig.Provide(NewUserService),
		dig.Invoke(StartService),
	)
}

func main() {
	app := InitApp()
	_ = app.Run(context.Background())
}
```

### 2. Add generate tag on top of main.go
```go
//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out dig_gen.go -unused error
package main
```

### 3. Generate dependency injection code
```bash
go generate ./...
```

## Supply Existing Instances
Inject pre-created objects directly with `Supply`, no wrapper constructor needed:
```go
func InitApp() *dig.App {
	globalCfg := Config{Addr: "127.0.0.1:8080"}
	return dig.Build(
		dig.Supply(globalCfg),
		dig.Provide(NewUserService),
	)
}
```
Support multiple values supply at once:
```go
dig.Supply(config, dbClient, zapLogger)
```

## Module Composition
Split code by business domain, assemble all modules in root app init function:
```go
// user/module.go
package user

import "github.com/shanjunmei/dig"

func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewRepository),
		dig.Provide(NewService),
		dig.Invoke(RegisterHttpRoute),
	)
}
```

```go
// main.go
func InitApp() *dig.App {
	return dig.Build(
		user.Module(),
		order.Module(),
		dig.Invoke(StartHttpServer),
	)
}
```

## Comparison with Mainstream DI Frameworks
| Comparison Item             | dig (This Project) | Google Wire          | Uber dig             | Uber FX              |
| --------------------------- | ------------------ | -------------------- | -------------------- | -------------------- |
| Code generation required    | &#9989; digen CLI        | &#9989; wire gen           | &#10060; No                | &#10060; No                |
| Runtime reflection          | &#10060; None at all      | &#10060; None at all       | &#9989; Heavy reflection  | &#9989; Heavy reflection  |
| Generation-time full validation | &#9989; Complete | &#9989; Complete | &#10060; Only runtime check | &#10060; Only runtime check |
| Closure Provider / Invoke   | &#9989; Native support  | &#9888;&#65039; Limited support   | &#10060; Not supported     | &#10060; Not supported     |
| Lifecycle start/stop hooks  | &#10060; Injection-only  | &#10060; No lifecycle      | &#10060; No lifecycle      | &#9989; OnStart / OnStop  |
| Unused constructor control  | 3 configurable modes | Silent drop only    | Silent ignore        | Silent ignore        |
| Business code external deps | Zero (stdlib only) | Zero                 | Multiple             | Multiple heavy deps  |
| Learning curve              | Very low (4 core APIs) | Medium verbose syntax | Medium tag & scope rules | High complex lifecycle system |
| Module declaration syntax   | Simple Module()    | Verbose NewSet()     | Manual function wrap | fx.Module bound with lifecycle |
| Final binary size           | Tiny, no reflection logic | Tiny | Medium | Largest (embed log/lifecycle/event bus) |
| Best application scenario   | Projects pursue zero runtime overhead & static safety | Large startup latency sensitive projects | Small simple apps accept reflection cost | Large microservices require full lifecycle management |

### Framework Selection Guide
1. Choose **dig**: You want static generation safety, zero runtime reflection overhead, native closure support, lightweight dependency.
2. Choose **Google Wire**: Accept verbose syntax, no demand for closure injection, only need basic static dependency management.
3. Choose **Uber dig**: Small simple project, acceptable runtime reflection, no lifecycle management requirement.
4. Choose **Uber FX**: System requires complete start/stop lifecycle hooks, plugin collection, tolerate slow startup and larger binary volume.

## Key Differentiation Summary
### VS Uber dig / Uber FX
Both frameworks rely on runtime reflection, all type & dependency checks run when program boots. Missing dependencies or unexported cross-package types will trigger online panic.
dig parses and validates the full dependency graph during `go generate`, all errors intercepted before deployment, zero reflection cost at runtime.

### VS Google Wire
Wire also uses code generation, but its syntax is redundant, closure support is limited, module combination logic cumbersome.
dig unifies all configurations under single `Option` interface, `Module` combination is smooth, auto-handle closure free variables with concise API design.

## CLI Flags for digen
| Flag      | Description |
| --------- | ----------- |
| `-out`    | Output generated file path, default `dig_gen.go` |
| `-unused` | Policy for unused constructors: `error` / `ignore` / `drop` |
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
Apache License 2.0, see `LICENSE` file in repository root.
