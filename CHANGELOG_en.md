# Changelog

All notable changes to `github.com/shanjunmei/dig` are documented in this file. For the Chinese version, see [CHANGELOG.md](./CHANGELOG.md).

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## ✨ Generation-time contract pre-check & diagnostics

- **Contract pre-check: `di.go` holds wiring only**  
  New `checkContractVisibility` in `internal/extractor/contract.go` runs **after** extraction but **before** any file is written. It scans the main package for package-level symbols (type/func/var/const) defined inside a `//go:build digen` file and checks whether any wiring (`dig.Provide` / `Invoke` / `Supply` / closure bodies) references one of them. On a hit it aborts generation (no file written) with a clear error and a `💡 Fix:`, naming the digen file and telling you to move the definition to a non-digen file or an imported package. The walk covers constructor signatures, `dig.Supply` value types, closure signatures and free-variable types, and merged parameter types, recursing through pointers/slices/maps/channels and generic type arguments. Added regression example `example/gen_failures/contract_digen_symbol/`.

- **Post-generation safety net now classifies contract violations vs. internal bugs**  
  `typeCheckGenerated` (added in v1.0.18) now classifies type errors in the generated file: if `undefined: X` refers to a main-package symbol defined in a digen file, it is reported as a **contract violation** (same `💡 Fix:` guidance as the pre-check); everything else is a genuine **internal generator bug** (pre-filled issue link). This closes the gap where an IR-cache hit (`-cache`) skips `BuildFinalNodes` and therefore the pre-check — the net is the last backstop for contract violations.  
  ⚠️ Fixed a previously misleading message: the internal-bug branch no longer claims "the symbol is most likely defined in your di.go" (if it were, the contract branch would have caught it); it now states plainly that this is a genuine internal bug and only suggests migration if you did define the symbol in a digen file.

- **Build-constraint checks consolidated into a shared package**  
  The duplicated implementations in `extractor` and `generator` (`fileHasDigenConstraint` / `genFileHasDigenConstraint`, `buildExprRequiresDigen` / `buildExprRequiresDigenGen`) are extracted into a dependency-free leaf package `internal/buildconstraint`, exposing `FileHasDigenConstraint(f *ast.File) bool` and `RequiresDigen(expr string) bool` as the single source of truth. All three call sites (`generator.go`, `extractor/buildtag.go`, `extractor/contract.go`) now use it. Removes duplicate logic and maintenance cost; added `internal/buildconstraint/buildconstraint_test.go`.

- **Golden-file regression test**  
  New `example/golden/golden_test.go`: at runtime it discovers each `example/*/dig_gen.go` that carries `//go:build digen` and a `//go:generate` directive, parses the directive to recover the exact flags, regenerates to a temp `-out`, strips the `//go:generate` meta line via `normalize()`, and byte-diff against the committed golden. Any silent output change (including "compiles but semantically drifted") is caught. Covers 11 complex examples, all byte-identical to their committed goldens; verified to catch drift (temporarily mutating a golden makes the test FAIL).

- **`digen` CLI subcommands**  
  Added `init` (scaffold a di.go with the dig.Build entry point), `check` (validate DI contracts without writing any file), `graph` (print the provider dependency graph as Mermaid), `explain <type>` (explain how a type/provider is resolved), and `completion <shell>` (bash/zsh/fish completion script). `init`/`check`/`graph`/`explain` reuse the generation pipeline and accept all the same flags; the `digen -h` help text and completion scripts list every flag and subcommand.

---

## [v1.0.18] - 2026-08-15

## ✨ Generation-time hardening & diagnostics

- **Providers may no longer declare a `context.Context` parameter**  
  Added `checkProviderContextParams`: if a provider (constructor registered via `Provide`, a closure, or `dig.Module`) declares a `context.Context` parameter, digen now rejects it at generation time with a `file:line:col` location and a `💡 Fix:`, aborting without writing a broken file.  
  Reason: providers are resolved eagerly inside `InitApp`, before the runtime `context.Context` exists, so the parameter would be undefined in the generated code; context injection is only valid inside `dig.Invoke` (which runs in the inner `func(ctx)`). Added regression example `example/gen_failures/provider_ctx/`.

- **`//go:build digen` is now validated at generation time**  
  Added `checkBuildSourceConstraint`: if a source file containing a `dig.Build` call lacks the `//go:build digen` constraint, digen errors at the `BuildFinalNodes` stage with a `💡 Fix:`, aborting generation (no file written). Reason: digen hard-codes `//go:build !digen` on the generated file, so without the matching tag on the source, a normal `go build` compiles both files and fails with `InitApp redeclared`. This promotes the long-standing "iron rule" from a doc convention into an invariant actually enforced by the generator.

- **Post-generation `go/types` safety net**  
  `internal/generator/generator.go` adds `typeCheckGenerated`, run after `format.Source` and before `os.WriteFile`. Because the user's source is already type-checked during loading (`packages.Load` with `NeedTypes` and `-tags=digen`, while `dig_gen.go` is excluded via `//go:build !digen`), any type error in the generated file is unconditionally an internal generator bug. On trigger: (1) no broken file is written; (2) the error is explicitly labeled an internal generator bug with the generated-file location; (3) a **one-click, pre-filled GitHub issue link** (`https://github.com/shanjunmei/dig/issues/new?title=...&body=...`) plus a copy-paste template is printed to drive the report upstream. If the type-check infrastructure itself fails, it best-effort falls back to writing the file normally.   The net only backstops unknown extraction/generation defects — it never replaces semantic rules and never masquerades as a user error.

- **`dig.Supply` of a module parameter no longer emits an undefined cross-package reference**  
  Fixed a generator bug where `dig.Supply(x)` whose value `x` is a function parameter or local variable of an inlined `dig.Module` was package-qualified (e.g. `user.cfg`), triggering `undefined: <pkg>.<param>` in the post-generation safety net. Such free variables are captured by the target function's own scope and are now referenced verbatim; only package-level symbols (var/func/const/type) are still qualified, preserving existing behaviour for cases like `db.Index` and `role.Config("production")`. The fix also prevents the inlined supply from pulling an unused import for the source package. Added regression example `example/supply_param/` (+ `example/supply_param_helper/`), auto-discovered by `example/successtest`.

- **AST-based type-name rewriting replaces regex rewriting**  
  Retired the fragile `replaceTypeNames` regex string rewriter (the word-boundary regex would wrongly rewrite same-named tokens inside string literals / comments, and bare-name matching could clobber same-named locals). It is replaced by a precise `go/ast` + `astutil.Apply` rewrite: first collect a `Pos -> "alias.Name"` rewrite plan keyed by `token.Pos` (explicitly skipping `SelectorExpr.Sel`), then apply it on a clone, touching only matched identifiers — never string literals, comments, or same-named locals. The `replacePkgPathWithAlias` path→alias rewrite for type strings is kept for backward compatibility. Generated output is behaviourally identical.

- **Stable serializable IR and optional disk cache**  
  The extractor→generator intermediate representation `[]model.Node` is now formalized as a serializable, schema-versioned stable IR: `internal/model` gains `CachedExtraction` (Nodes + ImportAliasMap/PkgAliasMap/PkgNameMap + `SchemaVersion`) and `Node` / `Arg` gain JSON tags; `UnmarshalJSON` errors on a `SchemaVer` mismatch instead of silently misreading. A new `internal/ir` package handles disk read/write (JSON by default, atomic write via temp file + rename), enabled by the `cmd/digen` `-cache` (default off) / `-cachedir` flags. When enabled, unchanged packages skip the expensive extraction / type-check step and reuse the cached IR; the cache key covers config knobs, `runtime.Version()`, and (recursively) the source hashes of the package and its transitive dependencies, so a dependency API change auto-invalidates the cache with no manual cleanup. Any cache-path failure gracefully falls back to re-extraction, and the cache is off by default so generation semantics are unaffected.

## 🐛 Bug Fixes (regressions introduced in v1.0.17)

- **Closure parameters / local variables falsely reported as `private` ("var X is private")**  
  The `checkFunctionVisibilityInClosure` added in v1.0.17 walks every bare-identifier call (`fn(args)` / `T(x)`) in a closure body before generation and runs `checkGenerationVisibility` on the call's function identifier. But `checkGenerationVisibility` also treats closure parameters (and local function / type variables) — which are `*types.Var` / `*types.TypeName` — as "package-level unexported symbols". When the closure is defined in a package **different from the generation target** (the classic cross-package module-inlining case `user.Module(cfg)`, where the closure lives in `user` but the target is `cmd/app`), `curPkg != mainPkgPath` and the name is unexported, so it is misclassified as "not visible across packages" and rejected — meaning **a perfectly valid closure-parameter call fails to generate**. `dig.Invoke(func(f func() Config){ _ = f() })` triggers exactly `var "f" is private`.  
  Fix (`internal/extractor/visibility.go`, commit `81e2a78`): the `*types.Var` branch of `checkGenerationVisibility` now returns early with `if !isPackageLevelVar(o) { return nil }`, validating only genuine package-level variables; the `*types.TypeName` case is guarded by the same `isPackageLevelVar` reasoning. Because the closure body is inlined into the target package, parameters and locals never constitute a cross-package reference and are always allowed. This also removes the false positives for legitimate cases beyond `private_visibility` / `closure_private_fn`.

## 📦 Examples and Documentation Updates

- **Automated regression tests for `gen_failures/`**  
  Added `example/gen_failures/gentest/gen_failures_test.go`, which walks each subdirectory, builds digen into a temp path, and asserts a non-zero exit plus a matching expected-error substring — so `provider_ctx` and other failure fixtures are caught by CI. Also added the `//go:build digen` tag (tag-only, no logic change) to four previously untagged fixtures (`ambiguous` / `cycle` / `missing_provider` / `named_mismatch`).
- Fixed an inaccurate "digen static check" claim in `prompts/system_prompt_dig.md` / `_en.md` — the source-file `//go:build digen` requirement is now actually enforced by digen at generation time.

---

## [v1.0.17] - 2026-08-13

## 🐛 Bug Fixes

- **Fix bare function/type calls inside closures not intercepting unexported cross-package symbols**  
  When the extractor lifts a closure from its original package into the generation target package, it prefixes bare identifier calls with the package qualifier. If the original package contains an unexported function (e.g. `buildAuditAuthorizer`), lifting produces an illegal `pkg.buildAuditAuthorizer` cross-package reference. Because `digen` does not compile the generated output, it silently exited 0 and emitted uncompilable code.

  Fix:
  - Added `checkFunctionVisibilityInClosure`: before generation, walks all bare identifier calls (`fn(args)` / `T(x)`) in the closure body and uses `checkGenerationVisibility` to reject unexported cross-package symbols (same-package, exported, and builtin symbols are allowed), producing a clear `private` error and no broken file
  - Extended `checkGenerationVisibility`'s switch with a `*types.TypeName` case to also cover unexported cross-package types in type conversions `T(x)`
  - Added regression example `example/gen_failures/closure_private_fn/`

- **Fix parameter shadowing in `example/db/db.go` `RedisClient.Ping`**  
  `Ping(index RedisDbIndex)` used the package-level variable `Index` instead of the `index` parameter in its `fmt.Printf`, making the parameter effectively unused. Now correctly references the `index` parameter.

- **Fix duplicate-binding error message for default Supply**  
  When two default (unnamed) `dig.Supply` calls provide the same type, the error message incorrectly showed `with name ""` instead of `(default)`. Root cause: in the unnamed case `keyNamed == keyDefault`, so the named-key check matched first. Refactored into `if instanceName != "" { ... } else { ... }` branches so the default-instance error correctly shows `(default)`.

## ♻️ Refactor and Optimisation

- **Generator config and sample adjustment**  
  Regenerated all example `dig_gen.go` files with the unified `dv` variable prefix, inline inlining enabled by default, and debug logging removed from generated code.
- **unused_provider example reworked**  
  `example/gen_failures/unused_provider` now registers its provider via the `dig.Provide` + `dig.Supply` pattern.

---

## 📦 Examples and Documentation Updates

- **New success examples**  
  `example/app_runtime_err` (runtime error-propagation paths: panic for provider errors, propagation for invoke errors) and `example/app_xpkg_generic` (cross-package generic `cache.Cache[*common.Config]` where both the generic type and the type argument come from different packages).
- **New failure examples**  
  Under `example/gen_failures/` added `ambiguous`, `duplicate_named`, `duplicate_supply`, `private_visibility`, `unused_provider`, covering named-instance ambiguity, duplicate named binding, duplicate default Supply, cross-package unexported visibility, and unused provider error paths.
- **GitHub Pages site**  
  Added `docs/` static site (project intro, core features, quick start, API overview, comparison matrix, examples, version timeline) plus a `.nojekyll` file.
- **GitHub Pages bilingual toggle**  
  Added 中文/EN language switching (localStorage persistence, synced title/meta, code blocks stay language-neutral).
- **System prompts & industrial coding-skill docs updated**  
  Trimmed and synced the files under `prompts/`.

---

## 🔧 Chores

- Bumped version references in README / CHANGELOG / system prompts to v1.0.17.

---

## [v1.0.16] - 2026-08-07

## ✨ New Features

- **ShadowGuard variable name shadowing protection**  
  Added a shadow protection mechanism that automatically detects and prevents variable name shadowing during code generation, improving the robustness of generated code.

- **Default behavior for unused parameters changed**  
  Changed the handling of unused parameters from the default `drop` (discard) to `error` (report an error), helping developers catch potential parameter omissions earlier.


## 🐛 Bug Fixes

- **Fix missing quotes for generic type parameters in logs**  
  Improved the generator's logging of Identity Target Type by properly wrapping type parameters with quotes, making log output more consistent and readable.

- **Fix identifier handling for cross‑package bare function calls**  
  Fixed an issue where the extractor incorrectly recognised identifiers when handling cross‑package bare function calls.

- **Fix free variable shadowing of package aliases**  
  Resolved a problem where free variable parameter names shadowed package aliases in non‑closure items. Adjusted the extraction logic to register package aliases earlier, ensuring that ShadowGuard correctly handles parameter lists and free variable mappings.

- **Fix shadowing caused by case‑sensitive variable names**  
  Renamed the package‑level variable `Db` to `db` to avoid naming conflicts with the imported `db` package alias, and updated all code references accordingly.


## ♻️ Refactor and Optimisation

- **Refactor ShadowGuard and free variable handling**  
  Refactored the shadowing logic for free variables and improved the ShadowGuard mechanism, enhancing the overall robustness of the code generator.

- **Enhanced metadata in generated code**  
  Added debug logs and clearer metadata information to the generated `dig_gen.go` files, making troubleshooting easier.

---

## 📦 Examples and Documentation Updates

- **Update Redis DB Index configuration example**  
  In the `example` module, added a new `RedisDbIndex` type and a default variable, updated the `Ping` method to receive and log the db index, and injected the db index via DI.

- **Update all generated dig_gen.go files**  
  Synchronised the regenerated `dig_gen.go` files across all example modules.

---

## 🔧 Chores

- Updated dependency versions and change logs
- Removed the `debug-aliases` flag and lifted the "one function per module" restriction
- Improved the third‑party library comparison matrix and cleaned up example code

---

## [v1.0.15] - 2026-08-03

### Fixed
- **Type package collection logic (core robustness fix)**:
  - `collectUsedPkgsFromType`: `*types.Named` branch incorrectly walked `TypeParams()` (declaration-level type params like `T any`, which have no real package path); now walks `TypeArgs()` (instantiation type args like `*common.Config`) while retaining `TypeParams()` walk for cross-package references inside constraints
  - Does NOT call `walk(t.Underlying())` on `*types.Named`: self-referential types (e.g. `type Node struct{ Next *Node }`) cause infinite recursion / stack overflow; cross-package references in struct fields / method signatures are covered by `collectTypeNameAndUsedPkgs` AST traversal
  - Added `*types.Signature` branch: function/method signature types (e.g. a provider returning `func(*common.Config) error`, or an invoke parameter being a function type) were previously not recursed into, so cross-package references inside signature params/results were not collected; generated code emitted full-path `func(*github.com/.../common.Config) error` instead of an alias, breaking compilation. Now walks `Recv`/`Params`/`Results`/`TypeParams` constraints, relying on `seen` for deduplication
  - `addPkgToUsed` changed from "`typePkg` single-value top-level package" to directly reuse `collectUsedPkgsFromType` full tree walk; no longer misses cross-package references inside Map key/value, slice elements, or generic type arguments
  - `allTypes` loop in `generateClosureDef` changed similarly: closure params / free vars / return types no longer rely on `typePkg` for only the top-level package; instead all cross-package references are tree-collected and `EnsureAlias`-ed individually

### Removed
- **Standalone `-debug-aliases` diagnostic flag**: Alias mapping diagnostics are now unified under the global `-debug` log (prints `[alias]` prefixed alias info automatically when `-debug` is on); no separate flag is provided anymore
- **`findExcludedPackagesInClosure` helper function**: The BFS transitive import closure naturally excludes "other main packages" and "other library packages containing `dig.Build`" (Go forbids importing main packages, and library packages are never imported twice); the extra exclusion logic was dead code, removed without affecting generated output
- **Single `dig.Module` per function restriction**: `findSingleModuleCall` → `findAllModuleCalls`; helper functions may now contain multiple `dig.Module` calls whose args are merged; Module calls inside control flow (if/switch/for/select) remain unsupported
- Removed `gen_failures/multi_module` and `gen_failures/multi_module_call` regression examples (no longer error cases)

### Changed
- **Third-party library comparison matrix expanded**: README and system prompts now include a complete dig / Wire / Fx comparison across architecture, API design, error handling, runtime/operations, and project status dimensions, plus per-tool trade-off analysis
- **`example/setup` cleanup**: Removed the redundant `digen` build tag from `full.go` and deleted `stub.go` (a placeholder only used in non-digen builds)

---

## [v1.0.14] - 2026-08-01

### Added
- **Closure inlining (`-inline` flag)**: Inline simple Provide/Invoke closures as immediately-invoked function expressions (IIFE), reducing generated function count (#16)
- **Identity closure optimization**: Closures of the form `func(p T) T { return p }` emit a direct type conversion instead of a wrapper function (#17)
- **Four identity-closure operation scenarios**: Introduced `OpKind` enum and refactored `analyzeIdentityClosure` to recognize direct return, address-of (`&`), dereference (`*`), and type conversion
- **Unified alias diagnostics via global `-debug` log**: When `-debug` is enabled, per-package import alias mappings are printed with the `[alias]` prefix automatically; no extra flag needed
- **Failure examples**: Added regression examples for 5 error types (`capture_const`, `capture_ctx`, `init_named_return`, `duplicate_provide`, `multi_module_call`) (#18)
- Source location (`file:line:col`) prefix added to all error messages across extractor/loader/processor, so failing providers/invokes/closures can be pinpointed without `-debug`
- Fixed bug where `item.Position` in `handleProvide` was swallowed by `ConditionalDebugf` (Position was always empty outside debug mode, causing `checkUnusedProviders` to report without a location)

### Fixed
- **`writeMainFunc` hardcoded `"context"`**: Ignored user-defined `context` import aliases (e.g., `import ctx "context"`), producing uncompilable code; now resolved via `getPkgAlias` (#20)
- **Cross-package alias leakage**: `digen ./...` vs `digen ./<pkg>` produced different output; `LoadImportAliases` now computes the transitive import closure via BFS and excludes other main / `dig.Build` packages (#21)
- **Context parameter name not propagated into closure bodies**: After the `writeMainFunc` fix, provider/invoke/identity-conversion code paths still hardcoded `"ctx"`; the resolved name is now threaded through `buildCallArgs`, `buildIdentityConversion`, `writeProvider`, `writeProviders`, `writeInvokes`, with `pickCtxParamName()` selecting a non-conflicting fallback (#22)
- **Inconsistent commit hash in `-version`**: `digen -version` truncated the hash to 8 characters via `shortCommit()`, while `mage install` showed the full 40-character hash; `shortCommit()` is removed and the full hash is emitted directly

### Changed
- Annotated unreachable error paths as safety nets; removed dead code (`checkExportedVisibility`, `checkFreeVarVisibility`, `model.Node.ShortName`) (#19)
- Simplified `findCycle` signature from `([]int, error)` to `[]int` (fallback returns `nil`)
- Renamed `BuildExecParams` to `buildExecParams` (internal)
- Closure inlining optimization enabled by default in generation config
- `checkGenerationVisibility`, `findSingleModuleCall`, `ValidateReturnType` now accept a `curPkg`/`fset` parameter and include source location in error messages
- Added `Position` field to `model.Node`, used by `checkUnusedProviders` for error reporting

**Closes #16, #17, #18, #19, #20, #21, #22**

---

## [v1.0.13] - 2026-07-30

### Added
- **Version info system**: Added `-version` CLI flag with ldflags injection and `git describe` parsing; added Mage build system (`mage build/install/test/vet`)
- **Provide closure signature validation**: Validates closure return signatures before code generation (only `(T)` or `(T, error)` allowed); illegal signatures are rejected with a clear error instead of generating uncompilable code (#11)
- **Generation failure test cases**: Added `gen_failures/` directory covering all error paths

### Fixed
- **Structured errors replacing panics**: All errors are returned as structured errors with package name, file location, and `💡 Fix:` suggestions; no more Go runtime panic stacks (#12)
- **Actionable error messages**: All error messages include scenario-specific `💡 Fix:` suggestions (e.g., missing Provider, name mismatch, circular dependency, unused Provider) (#13)
- **Always show detailed errors**: Detailed error info for failed packages is always shown; the `-debug` flag now only controls debug logs (#14)

**Closes #11, #12, #13, #14**

---

## [v1.0.12] - 2026-07-15

### Fixed
- **Errors not checked when provider results are ignored**: When generating wiring code with blank identifiers (i.e., `_ = provider()`), errors returned by providers were silently discarded, leading to potential runtime failures without panic.

  Fixed behavior:
  - If the provider returns no error, use `_ = fn()` as before.
  - If the provider returns an error, generate explicit error check:
    ```go
    if _, err := fn(args); err != nil {
        panic(err)
    }
    ```

  This aligns with the "fail-fast" principle and matches the error handling used for non-blank provider calls.

---

## [v1.0.11] - 2026-07-10

### Added
- **Named instance injection**: Inject multiple instances of the same type by distinguishing them via parameter names; useful for multiple DB connections, multiple Redis clients, etc.

### Fixed
- **Compilation errors from mismatched package name and import path**: The generator incorrectly used the last segment of the import path as the package reference name (e.g., using `v9` instead of `redis` for `github.com/redis/go-redis/v9`), leading to undefined identifier errors (`v9.Client`) in generated code. User-defined custom aliases in the main package (e.g., `loader "path"`) were also lost due to flawed alias collection logic.

  Root cause: `collectPkgAlias` relied on `importAliasMap`, but `loadImportAliases` did not preserve main-package aliases with priority; when no alias was found, the generator fell back to the last path segment instead of the actual package name (`pkg.Name`).

  Fix:
  - `collectPkgAlias` now scans the main package's AST for explicit import aliases first
  - If no explicit alias exists, uses the package's actual default name (`pkg.Name`)
  - Only when the default name conflicts does the generator invoke the alias strategy to produce a unique alternative

---

## [v1.0.10] - 2026-07-05

### Fixed
- **Generated code failed to compile**:
  - Missing alias replacement for standard library types (e.g., `*net/http.ServeMux`), causing `go/format` parsing errors
  - Missing imports for external parameters (e.g., `*config.AppConfig`) and closure signature types (e.g., `alias.AliasStrategy`)

  Root cause:
  - `collectPkgAlias` skipped standard library packages (`pkg.Module == nil`), leaving `net/http` etc. without aliases
  - `populateUsedPkgs` unconditionally overwrote `UsedPkgs`, clearing packages already set by `addExternalParams`
  - Closure generation (`generateClosureDef`) did not add type package paths to `usedPkgs`
  - `collectTypeNameAndUsedPkgs` failed to handle `*types.PkgName`, so package references inside closure bodies (like `alias.ParseAliasType`) were not recorded

  Fix:
  - Removed the `pkg.Module == nil` restriction to generate aliases for all non-main packages, including stdlib
  - Centralized `UsedPkgs` population in `populateUsedPkgs` with preservation of already-set values
  - `generateClosureDef` explicitly adds package paths of all involved types to `usedPkgs`
  - Restored handling of `*types.PkgName` in `collectTypeNameAndUsedPkgs`

---

## [v1.0.9] - 2026-07-01

### Added
- **`debugCommentf` helper**: Returns a formatted string only when the `-debug` flag is enabled; used to generate optional source comments in extracted items, keeping generated code cleaner when debug mode is off

---

## [v1.0.8] - 2026-06-25

### Changed
- **digen v2 promoted to primary generator**:
  - Renamed `cmd/digenv2` → `cmd/digen` as the current default generator
  - Moved old `cmd/digen` (v1) to `cmd/digenv1` for legacy compatibility
  - Updated all import paths and generation directives to reference the new location
  - "digen" is now the canonical name for the latest version

---

## [v1.0.7] - 2026-06-20

### Added
- **Generic argument support**:
  - Added `GenericArgsStr` field to `extractedItem`, storing the cleaned pure generic type segment `[T1, T2]`
  - Added `GenericArgs` field to `model.Node` to pass generic info to the code generator

### Changed
- Rewrote `extractGenericArgStr`: only parses `IndexExpr`/`IndexListExpr` indices, strips function identifier prefix
- Updated generator render logic for provider/unused-provider/invoke statements to append generic args
- Reuses `replacePkgPathWithAlias` for every generic sub-type, resolving cross-package full-path syntax issues

---

## [v1.0.6] - 2026-06-15

### Added
- **Function arguments as implicit `dig.Supply` providers**: The function containing `dig.Build` can now accept arguments, which are automatically registered as implicit Supply providers in the dependency graph.

  Key changes:
  - Added `addExternalParams` to register each function parameter as a Supply node
  - Generated code preserves the original function signature
  - Rejects `context.Context` parameters (should be provided by the caller via the returned function's argument)
  - Updated `generateCode`, `writeMainFunc`, and related functions to include parameter formatting and propagation

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

### Breaking Changes
- **Removed `*dig.App`**: `InitApp()` returns `func(context.Context) error`; generated code has zero runtime dependency
- **Upgrade guide**: replace `app.Run(ctx)` with `run := InitApp(); run(ctx)`

### Changed
- Refactor: eliminated runtime dependency by returning a bare closure
- Refactor: removed `App` interface definition and cleaned up related code
- Added: support for recursive package generation (`digen ./...`)
- Fixed: `writeMainFunc` parameter types
- Reverted: removed `/v2` suffix from module path

---

## [v1.0.4] - 2026-06-05

### Changed
- **Refactor: extracted alias strategies and functional helpers into `pkg/`**
  - Moved alias generation logic from `cmd/digen` to `pkg/alias`
  - Split strategies into separate files: `alias.go` (interface, short/full), `alias_obf.go` (obfuscated), `alias_numeric.go` (numeric)
  - Added `pkg/functional` with generic `Map`/`Reduce`/`Keys` utilities
  - Removed unused `alias.go`, `alias_obf.go`, `util.go` from `cmd/digen`

---

## [v1.0.3] - 2026-05-30

### Added
- **Original package path comment on generated closures**: When a closure (`__p_*`, `__i_*`) is moved to the current package, its original package path is lost in logs; added a comment above the closure definition to retain this information for debugging

---

## [v1.0.2] - 2026-05-25

### Fixed
- **Unused closure providers not removed in drop mode**: When using `-unused=drop`, closure definitions for unused providers were still emitted in generated code, leaving dead code behind.

  Fix:
  - Moved flag variables to `main()` to reduce package-level globals
  - Refactored `writeClosureDefs` to accept `refCount` and `unusedMode`, skipping unused closures when drop is enabled
  - Reused `refCount` across generation functions to avoid duplicate counting

  All three unused modes (error, ignore, drop) now behave consistently for both normal providers and closures.

---

## [v1.0.1] - 2026-05-20

### Changed
- Code optimization

---

## [v1.0.0] - 2026-05-15

### Added
- Initial release
