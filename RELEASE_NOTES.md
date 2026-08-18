# Release Notes

User-facing release notes for [`github.com/shanjunmei/dig`](https://github.com/shanjunmei/dig).

For the full, implementation-level changelog (internal file references, exact functions, commit hashes), see [CHANGELOG_en.md](./CHANGELOG_en.md) (English) / [CHANGELOG.md](./CHANGELOG.md) (中文).

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this project adheres to [Semantic Versioning](https://semver.org/).

---

## [v1.0.19] - 2026-08-18

> `digen` CLI subcommands + generation-time contract pre-check

### ✨ New Features

- **Five new `digen` subcommands** (reuse the generation pipeline, accept all existing flags):
  - `digen init` — scaffold a `di.go` with the `dig.Build` entry point
  - `digen check` — validate DI contracts without writing any file (lightweight static check)
  - `digen graph` — print the provider dependency graph as Mermaid
  - `digen explain <type>` — explain how a type / provider is resolved
  - `digen completion <shell>` — emit bash / zsh / fish completion scripts
  - `digen -h` and the completion scripts now list **every flag and subcommand**.

### 🔧 Improved Diagnostics

- **Generation-time contract pre-check**: ensures `di.go` holds wiring only. If any wiring references a package-level symbol defined inside a `//go:build digen` file, generation aborts (no file written) with a clear error and a `💡 Fix:` that names the file and tells you to move the definition to a non-digen file or an imported package. Covers constructor signatures, `dig.Supply` value types, closure signatures & free-variable types, merged parameter types, recursing through pointers / slices / maps / channels and generic type arguments.
- **Post-generation safety net upgraded**: now classifies **contract violations** vs **internal generator bugs**. Contract violations get the same `💡 Fix:` as the pre-check; genuine bugs emit a pre-filled issue link. Also fixes a previously misleading error message.

### ⬆️ Upgrade Notes

- This is a **largely additive, non-breaking release**. The only behavior change: the generation-time contract pre-check now catches a class of wiring that was already invalid *before* any file is written — a package-level symbol (type / func / var / const) defined inside a `//go:build digen` file and referenced by `dig.Provide` / `Invoke` / `Supply` or a closure body. Previously this only surfaced during the post-generation type check; now generation aborts immediately with a `💡 Fix:`.
- If you do reference a symbol defined in a digen file from your wiring, follow the `💡 Fix:` to move the definition to a file without the `//go:build digen` tag, or reference it from an imported package.
- Run `digen check` for a pure static contract check before triggering a full generation.

### 🛠 Internals & Quality (for contributors)

- Build-constraint checks consolidated into a single source of truth, `internal/buildconstraint` (`FileHasDigenConstraint` / `RequiresDigen`), removing duplicated logic.
- New golden-file byte-diff regression test (`example/golden/golden_test.go`) covering 11 complex examples, catching silent "compiles but semantically drifted" changes.

---

## [v1.0.18] - 2026-08-15

> Generation-time hardening & diagnostics

### ✨ New Features

- **Providers may no longer declare a `context.Context` parameter** — digen rejects it at generation time with a `file:line:col` location and a `💡 Fix:`, because providers resolve eagerly inside `InitApp` before any runtime `context.Context` exists. Context injection stays valid only inside `dig.Invoke`.
- **`//go:build digen` is now enforced at generation time** — if the source file with `dig.Build` lacks the constraint, digen errors before writing any file. This turns the long-standing "iron rule" into a generator-enforced invariant (previously a normal `go build` would fail with `InitApp redeclared`).
- **Post-generation `go/types` safety net** — after generation, the produced file is type-checked; any error is unambiguously an internal generator bug, and digen prints a one-click, pre-filled GitHub issue link instead of writing a broken file.

### 🐛 Bug Fixes

- `dig.Supply` of a module parameter no longer emits an undefined cross-package reference.
- AST-based type-name rewriting replaces the old regex rewriter, fixing incorrect rewrites inside string literals / comments and same-named locals.
- **Stable serializable IR + optional disk cache** — the extractor→generator IR is now schema-versioned and cacheable to disk via `-cache` / `-cachedir` (off by default); unchanged packages skip re-extraction, and the cache auto-invalidates on any dependency or config change.
- Fixed a v1.0.17 regression where closure parameters / local variables were falsely reported as `private` ("var X is private") under cross-package module inlining.

### 📦 Examples & Docs

- Automated regression tests for `gen_failures/` (CI now catches failure fixtures).
- Fixed an inaccurate "digen static check" claim in the prompts — the `//go:build digen` requirement is now actually enforced.

---

## [v1.0.17] - 2026-08-13

### 🐛 Bug Fixes

- **Unexported cross-package symbols inside closures are now intercepted** — when a closure is lifted into the generation target, bare calls to unexported functions/types from the original package produced illegal cross-package references that digen silently emitted (uncompilable code). Now rejected with a clear `private` error and no broken file.
- Fixed parameter shadowing in `example/db` `RedisClient.Ping` (used the package-level `Index` instead of the `index` parameter).
- Fixed duplicate-binding error message for default `dig.Supply` (now shows `(default)` instead of `with name ""`).

### ♻️ Refactor & Optimisation

- Regenerated all example `dig_gen.go` with the unified `dv` prefix, inlining enabled by default, debug logging removed from generated code.
- `unused_provider` example reworked to use the `dig.Provide` + `dig.Supply` pattern.

### 📦 Examples & Docs

- New success examples: `app_runtime_err` (error-propagation paths) and `app_xpkg_generic` (cross-package generic).
- New failure examples: `ambiguous`, `duplicate_named`, `duplicate_supply`, `private_visibility`, `unused_provider`.
- Added the `docs/` GitHub Pages site (bilingual 中文/EN toggle) with project intro, features, quick start, API overview, comparison matrix, examples, and version timeline.
- System prompts & industrial coding-skill docs updated.

### 🔧 Chores

- Bumped version references in README / CHANGELOG / system prompts to v1.0.17.

---

## [v1.0.16] - 2026-08-07

### ✨ New Features

- **ShadowGuard variable-name shadowing protection** — automatically detects and prevents variable shadowing during code generation.
- **Unused parameters now error by default** (was `drop`) — helps catch omitted parameters earlier.

### 🐛 Bug Fixes

- Fixed missing quotes for generic type parameters in logs.
- Fixed identifier handling for cross-package bare function calls.
- Fixed free-variable shadowing of package aliases in non-closure items.
- Fixed shadowing caused by case-sensitive variable names (`Db` → `db`).

### ♻️ Refactor & Optimisation

- Refactored ShadowGuard and free-variable handling.
- Added debug logs / clearer metadata to generated `dig_gen.go`.

### 📦 Examples & Docs

- Added `RedisDbIndex` type and DI-injected db index in the example module.
- Regenerated all `dig_gen.go` across examples.

### 🔧 Chores

- Updated dependency versions; removed the `debug-aliases` flag; lifted the "one function per module" restriction; improved the comparison matrix.

---

## [v1.0.15] - 2026-08-03

### 🐛 Fixed

- **Type package collection robustness** — fixed infinite recursion on self-referential types, missing cross-package references inside function signatures / map / slice / generic type arguments, and signature types not being recursed. Generated code now uses correct aliases and compiles.
- Removed dead code (`findExcludedPackagesInClosure`).

### ➖ Removed

- Standalone `-debug-aliases` flag (merged into global `-debug`).
- Single `dig.Module` per function restriction — helper functions may now contain multiple `dig.Module` calls (args merged); Module calls inside control flow remain unsupported.
- Removed `multi_module` / `multi_module_call` regression examples (no longer error cases).

### 🔄 Changed

- Comparison matrix expanded to cover architecture, API design, error handling, runtime/operations, and project status across dig / Wire / Fx.
- `example/setup` cleanup (removed redundant build tag and placeholder stub).

---

## [v1.0.14] - 2026-08-01

### ➕ Added

- **Closure inlining (`-inline`)** — inline simple Provide/Invoke closures as IIFEs, reducing generated function count.
- **Identity-closure optimization** — `func(p T) T { return p }` emits a direct type conversion (4 scenarios recognized: return, address-of, dereference, conversion).
- Unified alias diagnostics via global `-debug` log.
- Failure examples for 5 error types (`capture_const`, `capture_ctx`, `init_named_return`, `duplicate_provide`, `multi_module_call`).
- **Source location (`file:line:col`) on all errors** — failing providers/invokes/closures are pinpointable without `-debug`.

### 🐛 Fixed

- `writeMainFunc` hardcoded `"context"` ignored user-defined `context` aliases.
- Cross-package alias leakage between `digen ./...` and `digen ./<pkg>`.
- Context parameter name now propagated into closure bodies.
- Inconsistent commit hash in `-version` (now full hash).

### 🔄 Changed

- Closure inlining enabled by default; various internal signature/naming cleanups.

---

## [v1.0.13] - 2026-07-30

### ➕ Added

- **Version info system** — `-version` CLI flag (ldflags injection + `git describe`); Mage build system (`build`/`install`/`test`/`vet`).
- **Provide closure signature validation** — only `(T)` or `(T, error)` allowed; illegal signatures rejected with a clear error.
- **Generation failure test cases** — `gen_failures/` covering all error paths.

### 🐛 Fixed

- **Structured errors replace panics** — all errors return structured info with package, file location, and `💡 Fix:` suggestions; no more Go panic stacks.
- Detailed errors always shown; `-debug` now only controls debug logs.

---

## [v1.0.12] - 2026-07-15

### 🐛 Fixed

- **Provider result errors were silently discarded** when wiring used blank identifiers (`_ = provider()`). Now, if a provider returns an error, digen generates an explicit check (`if _, err := fn(args); err != nil { panic(err) }`), matching fail-fast behavior.

---

## [v1.0.11] - 2026-07-10

### ➕ Added

- **Named instance injection** — inject multiple instances of the same type via parameter names (e.g., multiple DB connections, multiple Redis clients).

### 🐛 Fixed

- **Package-name vs import-path mismatch compilation errors** — the generator used the last import-path segment as the reference name (e.g., `v9` instead of `redis` for `go-redis/v9`). Now scans the main package's AST for explicit aliases first, falls back to the actual package name, and only then uses an alias strategy.

---

## [v1.0.10] - 2026-07-05

### 🐛 Fixed

- **Generated code failed to compile** — missing alias replacement for stdlib types (e.g., `*net/http.ServeMux`), missing imports for external parameters and closure signature types.
  - Now generates aliases for all non-main packages (incl. stdlib); `UsedPkgs` population centralized with preservation of already-set values; closure generation adds involved type packages; restored `*types.PkgName` handling.

---

## [v1.0.9] - 2026-07-01

### ➕ Added

- **`debugCommentf` helper** — emits optional source comments in extracted items only when `-debug` is enabled, keeping generated code clean otherwise.

---

## [v1.0.8] - 2026-06-25

### 🔄 Changed

- **digen v2 promoted to primary generator** — `cmd/digenv2` renamed to `cmd/digen` (default); old v1 moved to `cmd/digenv1` for legacy compatibility. "digen" now refers to the latest version.

---

## [v1.0.7] - 2026-06-20

### ➕ Added

- **Generic argument support** — providers/invokes can carry generic type arguments; render logic appends them, reusing alias rewriting for each subtype.

### 🔄 Changed

- Rewrote generic-arg extraction (parses `IndexExpr`/`IndexListExpr` only); updated generator render for provider/unused-provider/invoke.

---

## [v1.0.6] - 2026-06-15

### ➕ Added

- **Function arguments as implicit `dig.Supply` providers** — the function containing `dig.Build` can accept arguments, auto-registered as Supply nodes. Generated code preserves the original signature; `context.Context` params are rejected (provide via the returned closure's argument).

  Example:

  ```go
  func InitApp(opts *deduper.Options) func(context.Context) error {
      return dig.Build(
          dig.Provide(storage.NewStore),
          dig.Provide(deduper.NewDeduper),
      )
  }
  ```

---

## [v1.0.5] - 2026-06-10

### ⚠️ Breaking Changes

- **Removed `*dig.App`** — `InitApp()` now returns `func(context.Context) error`; generated code has zero runtime dependency.
- **Upgrade guide**: replace `app.Run(ctx)` with `run := InitApp(); run(ctx)`.

### 🔄 Changed

- Eliminated runtime dependency by returning a bare closure; removed the `App` interface.
- Added recursive package generation (`digen ./...`).
- Reverted: removed `/v2` suffix from the module path.

---

## [v1.0.4] - 2026-06-05

### 🔄 Changed

- **Refactor: extracted alias strategies and functional helpers into `pkg/`** — alias generation moved to `pkg/alias` (interface + short/full/obfuscated/numeric strategies); added `pkg/functional` with generic `Map`/`Reduce`/`Keys`; removed unused files from `cmd/digen`.

---

## [v1.0.3] - 2026-05-30

### ➕ Added

- **Original package-path comment on generated closures** — a comment above each lifted closure retains its source package path for debugging.

---

## [v1.0.2] - 2026-05-25

### 🐛 Fixed

- **Unused closure providers not removed in drop mode** — with `-unused=drop`, dead closure definitions were still emitted. Now `writeClosureDefs` skips unused closures; all three unused modes (error/ignore/drop) behave consistently for providers and closures.

---

## [v1.0.1] - 2026-05-20

### 🔄 Changed

- Code optimization.

---

## [v1.0.0] - 2026-05-15

### ➕ Added

- Initial release.
