# dig Core Skill

> "Core usage" submodule of `system_prompt_dig_en.md`, for the `github.com/shanjunmei/dig` compile-time DI library.
> Covers APIs, syntax rules, the digen CLI and standard templates. Historical version diffs live in `dig-migration_en.md`; troubleshooting in `dig-troubleshooting_en.md`; cross-comparison in `dig-comparison_en.md`.

## 1. Basic Info

- Positioning: compile-time IoC container based on code generation, zero runtime reflection, zero runtime dependency on dig after generation
- Go version: 1.25+
- Default generated filename: `dig_gen.go` (not `di_gen.go`)
- License: MIT

```bash
go get github.com/shanjunmei/dig@latest
go install github.com/shanjunmei/dig/cmd/digen@latest
```

## 2. Five Core APIs

1. `dig.Build(opts ...Option)`: Assemble DI container, return startup function
2. `dig.Provide(constructors ...any)`: Register constructors
3. `dig.Supply(values ...any)`: Inject arbitrary runtime/constant values (breaks Wire's constant-only limit)
4. `dig.Invoke(functions ...any)`: Execute startup logic after dependencies resolved, supports error return
5. `dig.Module(opts ...Option)`: Group options for reusable, nested modules with duplicate detection

## 3. Named Instance Injection

- Define: `dig.Provide(func() (mainDB, reportDB *sql.DB, err error) {...})` or `dig.Supply(mainDB)` (variable name becomes instance name)
- Consume: `dig.Invoke(func(mainDB *sql.DB) {...})` (parameter name matches instance name)
- Ambiguity: multiple instances exist but consumer doesn't specify parameter name → ambiguous dependency error listing available names

## 4. Mandatory Syntax Restrictions

1. **Closure capture rule**: Provide/Invoke anonymous closures cannot capture local variables inside InitApp; only package-level variables and literals allowed
2. **DI config isolation**: Source `di.go` files SHOULD carry `//go:build digen` (convention): digen only recognizes the `di.go` entrypoint and does not parse every source file; `go build` skips files with this tag, avoiding a duplicate `InitApp` declaration at normal build. The generated `dig_gen.go` is written by digen with `//go:build !digen` (hardcoded, safe). Do NOT define business structs, constructors, custom types, or global constants here. Source files carrying `//go:build digen` are validated by digen at generation time
3. **Primitive type conflicts**: Use wrapper types (e.g. `type UseMySQL bool`)
4. **Generic instantiation**: Must explicitly instantiate, e.g. `dig.Provide(NewStore[int])`
5. **Conditional branches**: Runtime if allowed inside closures; top-level if wrapping Module() forbidden (all branches register simultaneously), use build tags for compile-time switching
6. **InitApp params**: Automatically registered as Supply values, no manual capture needed

## 5. digen CLI

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | dig_gen.go | Generated filename, ignored under `digen ./...` |
| `-unused` | error | Unused constructor policy: error / ignore / drop |
| `-debug` | false | Debug logs (includes alias mapping diagnostics) |
| `-alias` | full | Alias strategy: full / short / obfuscated / numeric |
| `-inline` | false | Inline simple closures as IIFEs, identity closures collapse to type conversion |
| `-version` | false | Print version information |
| `-typecheck` | true | Type-check generated code after emission to catch internal generator bugs (disable for large `./...` runs) |
| `-cache` | false | Cache the extracted IR to disk; unchanged packages skip extraction / type-checking |
| `-cachedir` | "" | IR cache directory (default `os.TempDir()/digen-ir-cache`; only with `-cache`) |

Subcommands: `init` (scaffold di.go), `check` (validate without writing), `graph` (Mermaid dependency graph), `explain <type>` (resolution path), `completion <shell>` (bash/zsh/fish completion script).

## 6. Standard Templates

```go
//go:build digen
package main

import (
    "context"
    "github.com/shanjunmei/dig"
)

func InitApp() func(context.Context) error {
    return dig.Build(
        dig.Provide(NewConfig),
        dig.Provide(NewDB),
        dig.Supply(DefaultTimeout),
        dig.Provide(func(t Timeout) *Server { return NewServer(t) }),
        dig.Invoke(func(srv *Server) error { return srv.Run() }),
    )
}
```

```bash
digen ./...
go run .
```

```go
// Override runtime Logf (dig_gen.go auto-generates global Logf)
func main() {
    Logf = log.Printf // replace with zap/logrus
    run := InitApp()
    if err := run(context.Background()); err != nil { panic(err) }
}
```

## 7. Forbidden Behaviors

1. Never confuse `go.uber.org/dig` (runtime DI) with `shanjunmei/dig` (compile-time DI)
2. Do not use Wire/Fx exclusive APIs in dig code
3. Do not provide examples violating closure capture restrictions
4. Do not fabricate non-existent APIs or digen flags
5. Never claim dig doesn't support multi-instance injection (named parameters are supported)
