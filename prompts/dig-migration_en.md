# dig Migration & Version Skill

> "Migration / version" submodule of `system_prompt_dig_en.md`. The single home for historical version diffs, so the main docs and core skill never duplicate them.

## 1. Upgrade from v1.0.4

- Old: `app.Run(ctx)`
- New: `run := InitApp(); run(ctx)`
- (Since v1.0.5, `InitApp()` returns `func(context.Context) error`; `*dig.App` removed.)

## 2. Version Change Summary

| Version | Key Changes |
|---------|-------------|
| v1.0.5 | Removed `*dig.App`, `InitApp()` returns `func(context.Context) error` |
| v1.0.11 | Named instance injection; package alias resolution fix (e.g. `go-redis/v9`) |
| v1.0.13 | `-version`/Mage build; Provide closure signature validation; structured errors with `💡 Fix:` |
| v1.0.14 | Closure inlining `-inline`; cross-package alias isolation; errors with `file:line:col`; global Logger unified diagnostics |
| v1.0.15 | Type package collection robustness fix; removed `-debug-aliases` (merged into `-debug`); lifted single Module restriction |
| v1.0.16 | ShadowGuard variable-shadowing protection; unused params default to `error` |
| v1.0.17 | Intercept unexported cross-package calls in closures; added examples and GitHub Pages site |
| v1.0.18 | Provider `context.Context` ban; `//go:build digen` generation-time check; `go/types` safety net; IR cache |
| v1.0.19 | `digen` CLI subcommands (init/check/graph/explain/completion); generation-time contract pre-check; type-check safety net (contract-violation vs internal-bug classification); build-constraint checks consolidated into `internal/buildconstraint`; golden-file regression test |

## 3. Current Key Features (by introduction version, for migration decisions)

- Named instance injection (v1.0.11+)
- Version info system (v1.0.13+): `-version`, Mage, structured errors
- Closure inlining (v1.0.14+): `-inline`
- Multi-Module support (v1.0.15+)
- ShadowGuard (v1.0.16+)
- Unexported cross-package call interception (v1.0.17+)
- Generation-time hardening & diagnostics (v1.0.18+)

## 4. Migrating Wire / Fx to dig

1. Use the comparison table (see `dig-comparison_en.md`) to confirm capability mapping
2. Rewrite runtime `fx.Provide`/`fx.Invoke` or `wire.NewSet` into `dig.Provide`/`dig.Invoke` inside `dig.Build(...)`
3. Interface binding: Wire `wire.Bind` / Fx `fx.As` → dig identity closure (inlined to type conversion)
4. Multi-instance: Fx named/value groups → dig named parameters
5. Drop runtime deps: remove `go.uber.org/fx`, `google/wire` imports and runtime startup code such as `app.Run`
6. Add `//go:build digen` to di.go and run `digen ./...` to generate `dig_gen.go`
