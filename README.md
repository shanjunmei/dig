# dig — Compile-time Code Generation Zero-Reflection DI Container for Go
[&#20013;&#25991;&#25991;&#26723;](./README_zh.md) | English Docs

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Core Philosophy
Compile-time static safety, zero runtime reflection overhead, no runtime magic, fully native Go style.
All dependency resolution logic is generated into static Go code at build time, runtime only executes plain function calls.

## Core Advantages
1. **Zero runtime reflection**
All dependency assembly logic is generated as static Go source code via `digen` CLI. No runtime type parsing, no reflection performance loss, startup performance equals handwritten initialization code.

2. **Mutually exclusive build tag 3-file isolation (official standard spec)**
Split project code into 3 independent files with exclusive build tags to eliminate duplicate definition / undefined symbol compile errors:
- `di.go` Tag `digen`: Only parsed by digen generator, excluded from normal build & run
- `main.go` No build tag: Shared base types, global variables, business models, entry `main()`, compiled in all scenarios
- `di_gen.go` Tag `!digen`: Auto-generated runtime entry file, skipped during code generation

3. **Strict separation rules for Supply & Provide closures (core constraint)**
- `dig.Supply`: Natively accepts package-level global runtime variables as type providers
- Inline `dig.Provide(func() T)` anonymous closure: Only constant literals are allowed as free variables; capturing local variables inside `InitApp()` will trigger generator error, avoid undefined symbol after code split

4. **Wrapper type solves primitive type injection conflict**
Direct injection of raw `bool`/`string`/`time.Duration` will cause duplicate provider conflicts. Define lightweight alias wrapper types for each business scenario, fully recognized and parsed by digen.

5. **Native Invoke post-start callback**
Dedicated `dig.Invoke()` API, all callback logic will be scheduled after all Supply & Provider instances are created. Designed for route registration, database initialization, service startup and other boot logic.

6. **Earlier dependency error detection**
Missing dependencies, circular dependencies, duplicate providers are fully validated during `go generate`; errors are exposed at development time instead of panicking online.

7. **3 configurable unused provider policies**
Support 3 modes to handle unused constructors via command flag `--unused`:
- `error`: Generation fails and throws error if unused provider exists (default)
- `ignore`: Keep unused provider with blank assignment `_ = fn()`
- `drop`: Remove unused provider code directly from generated file

8. **Minimal external dependencies**
Core library only relies on Go standard library; digen generator only depends on official `golang.org/x/tools` AST toolkit, no heavy third-party dependencies.

9. **Low learning cost**
Only 4 core APIs: `dig.Build`, `dig.Provide`, `dig.Supply`, `dig.Invoke`; clear build tag & closure constraints, low entry barrier.

10. Tiny final binary size
No embedded reflection runtime, no redundant framework logic, compiled binary size is close to handwritten initialization code.

## Mandatory Build Tag Isolation Rules (Must Follow)
### File Split & Tag Matching Rules
1. `di.go` (Generator-only init logic, tag `digen`)
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
		// Supply accepts global variables defined in main.go
		dig.Supply(s),

		// Provide closure only uses constant literals, no outer local variable capture
		dig.Provide(func() EnableCache { return EnableCache(true) }),
		dig.Provide(func() QueryTimeout { return QueryTimeout(3 * time.Second) }),

		dig.Provide(NewConfig),
		dig.Provide(func(t QueryTimeout, um UseMySQL, rm EnableCache) any {
			if um {
				return NewMySQLStore(t)
			}
			return NewRedisStore(rm)
		}),
		dig.Provide(NewServer),

		// All boot logic executes after all instances are ready
		dig.Invoke(func(srv *Server, um UseMySQL, ec EnableCache, t QueryTimeout) {
			println("service boot complete")
			println("mysql switch:", bool(um))
			println("cache enable:", bool(ec))
			println("query timeout sec:", t.Seconds())
			_ = srv.Run()
		}),
	)
}
```
- Only contains `InitApp()` with full `dig.Build()` chain
- Only parsed by digen during `go generate`
- Skipped during normal `go run` / `go build` due to tag mismatch

2. `main.go` (Shared base code, no build tag)
```go
package main

import (
	"context"
	"time"
)

// Global runtime variable consumed by dig.Supply
var s = UseMySQL(true)

// Unique wrapper types to resolve primitive type injection collision
type UseMySQL bool
type EnableCache bool
type QueryTimeout time.Duration

// Business model & constructors
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
func NewServer(store any) *Server {
	return &Server{store: store}
}
func (s *Server) Run() error { return nil }

func main() {
	app := InitApp()
	_ = app.Run(context.Background())
}
```
- All custom wrapper types, business structs, global variables, program entry `main()`
- Compiled under all build modes, visible to both generator and runtime

3. `di_gen.go` (Auto-generated runtime entry, tag `!digen`)
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

// Auto-split top-level functions generated by digen for closures
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
	println("service boot complete")
	println("mysql switch:", bool(um))
	println("cache enable:", bool(ec))
	println("query timeout sec:", t.Seconds())
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
- Automatically overwritten after every `go generate`
- Provides runtime `InitApp()` implementation for online execution
- Excluded during code generation due to conflicting `!digen` tag with generator `--tags=digen`

### Consequence of Incorrect Tag Configuration
Mismatched tags or mixed file logic will trigger fatal compile errors:
1. Duplicate definition: Two `InitApp()` functions exist in one package
2. Undefined symbol: Generator cannot resolve base types / global variables
3. Broken dependency topology parsing inside digen

## Installation Guide
### 1. Install core library to project
```bash
go get github.com/shanjunmei/dig
```

### 2. Install digen code generation CLI tool
```bash
go install github.com/shanjunmei/dig/cmd/digen@latest
```
Minimum Go version requirement: Go 1.21+

## Generation & Runtime Commands
```bash
# Generate DI static code
go generate ./...
# Directly call digen tool
digen -out di_gen.go -unused=drop --tags=digen

# Normal compile & run program
go run .
```

## Horizontal Comparison With Mainstream Go DI Frameworks
| Comparison Item | dig (This Project) | Google Wire | Uber dig (Reflection Runtime) | Uber FX (Reflection Runtime) |
| ---- | ---- | ---- | ---- | ---- |
| Require code generation | &#9989; digen CLI tool | &#9989; wire gen | &#10134; No code generation workflow | &#10134; No code generation workflow |
| Runtime reflection exists | &#10060; Zero, pure static generated code | &#10060; Zero, pure static generated code | &#9989; Heavy runtime reflection | &#9989; Heavy runtime reflection |
| Full dependency validation at generation time | &#9989; Full link check (missing/circular dependency error at dev stage) | &#9989; Full link check at generate time | &#10134; No generate step, only dynamic runtime check | &#10134; No generate step, only dynamic runtime check |
| Mutually exclusive build tag 3-file isolation spec | &#9989; Official standard, native adapted by digen | &#9989; Natively support build tag, self-implementable 3-file isolation | &#10134; No code generation, no duplicate InitApp conflict, no need this spec | &#10134; No code generation, no duplicate InitApp conflict, no need this spec |
| Supply native support for package global runtime variables | &#9989; Top-level `dig.Supply` built-in API, out-of-box | &#9888;&#65039; Global var injection supported, no dedicated Supply syntax, verbose writing | &#10060; No matching API, only constructor parameter injection | &#10060; No matching API, only constructor parameter injection |
| Provide closure constraint: Only constant literals allowed, forbid capturing outer local variables | &#9989; Tool forced validation, error thrown if capture local variables | &#9888;&#65039; No forced validation, arbitrary free variable capture allowed; risk of undefined symbol after generation | &#10134; Anonymous closure Provide syntax unsupported | &#10134; Anonymous closure Provide syntax unsupported |
| Wrapper type resolve primitive type injection collision | &#9989; Official recommended standard solution, fully parsed by generator | &#9888;&#65039; Syntactically supported, manual type wrapping required, no auxiliary generator check | &#10060; No built-in differentiation logic, identical primitive types cannot be distinguished | &#10060; No built-in differentiation logic, identical primitive types cannot be distinguished |
| Native Invoke post-boot callback | &#9989; Built-in `dig.Invoke` API dedicated to route register / service startup | &#9888;&#65039; No native API, massive boilerplate code required for manual encapsulation | &#10060; Independent Invoke callback mechanism missing | &#9989; Full built-in lifecycle system (Start/Hook) |
| Unused constructor control policy | 3 configurable modes: error / ignore / drop (controllable at generate time) | &#9888;&#65039; Silent drop only, no error alert mode | &#10134; Runtime framework, no generate-time control logic, silent ignore | &#10134; Runtime framework, no generate-time control logic, silent ignore |
| External dependency count of business core library | Zero, only Go standard library | Zero, only Go standard library | Multiple third-party dependencies | Massive heavy dependencies (log, event bus, full lifecycle components) |
| Learning curve | Extremely low: only 4 core APIs + clear tag / Supply constraints | Medium: simple base API, verbose closure & global var writing, many hidden pitfalls | Medium: need master type tag & scope rules | High: complex full lifecycle & module layered system |
| Final compiled binary size | Tiny, no embedded reflection runtime logic | Tiny, no embedded reflection runtime logic | Medium, embedded reflection dispatch code | Largest, built-in log, event bus, full lifecycle components |

### Symbol Annotation Explanation
1. `&#9989;`: Framework natively provides official standard API/spec, out-of-box, no extra adaptation cost
2. `&#9888;&#65039;`: Syntactically implementable, but tedious workflow with potential compile/runtime risks, no official simplified solution
3. `&#10060;`: Framework underlying does not support this capability, no corresponding implementation logic
4. `&#10134;`: Framework positioned as reflection-based runtime DI, no code generation workflow. This item is exclusive spec for code-generation tools; the framework naturally does not require this capability, not a functional defect.

### Key Correction Description For Comparison Table
1. Build tag 3-file isolation
This spec is exclusive engineering standard for code-generation DI tools, used to separate generator parsing source (`digen` tag) and online runtime source (`!digen` tag), avoid duplicate definition of `InitApp()`.
Google Wire natively supports Go build tag, developers can implement identical 3-file isolation manually.
Uber dig / FX have no code generation step, no two copies of `InitApp()` to conflict, so this isolation spec is unnecessary. The original table mark "not supported" is misleading and revised.

2. Runtime framework related items unified semantic adjustment
All code-generation exclusive features (generate-time validation, 3-file isolation, unused constructor control) are marked `&#10134;` for Uber dig/FX, clarify it's positioning difference rather than function missing.

3. Objective description for Wire Supply & closure capture
Wire can pass global variables as dependencies, but has no independent top-level Supply function, writing is verbose. Meanwhile Wire has no free variable capture validation; capturing local variables inside InitApp will lead to `undefined` compile error in generated file, the risk mark is retained objectively.

4. Invoke capability boundary distinction
dig's `Invoke` is lightweight post-creation callback without heavy lifecycle overhead; FX provides complete lifecycle hook system. The two design targets are different, only objective description of capability form, no subjective pros and cons judgment.
