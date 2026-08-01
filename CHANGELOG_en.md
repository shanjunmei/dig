# Changelog

All notable changes to `github.com/shanjunmei/dig` are documented in this file. For the Chinese version, see [CHANGELOG.md](./CHANGELOG.md).

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/).


---

## [v1.0.14] - 2026-08-01

### Added
- **Closure inlining (`-inline` flag)**: Inline simple Provide/Invoke closures as immediately-invoked function expressions (IIFE), reducing generated function count (#16)
- **Identity closure optimization**: Closures of the form `func(p T) T { return p }` emit a direct type conversion instead of a wrapper function (#17)
- **Four identity-closure operation scenarios**: Introduced `OpKind` enum and refactored `analyzeIdentityClosure` to recognize direct return, address-of (`&`), dereference (`*`), and type conversion
- **`-debug-aliases` diagnostic flag**: Prints the resolved per-package import alias mapping during generation
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
