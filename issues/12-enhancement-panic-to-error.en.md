# Enhancement: Replace Panics with Structured Error Returns

## Description

In `internal/extractor/extractor.go`, multiple locations used `panic()` to handle error conditions. When encountering invalid user code (e.g., circular dependencies, missing providers), the program would crash directly with a Go runtime panic stack trace, which is unfriendly to users and cannot be caught and formatted by upper layers.

## Affected Locations

In `internal/extractor/extractor.go`, the following locations used `panic`:

| Function | Line (approx.) | Trigger Condition |
|---|---|---|
| `resolveArgNames` | ~1315 | Provider not found for argument |
| `buildInvokeNode` | ~1690 | Invoke closure code generation failed |
| `buildProviderNode` | ~1975 | Provider closure code generation failed |

These panics were triggered on invalid user input (e.g., missing Provider) or system errors (e.g., code generation failure).

## Solution

Replaced all `panic` calls with propagatable `error` return values, enabling errors to be captured and formatted by upper layers (`buildFinalNodes` → `buildNodes` → `processPackage` → `processor.Process` → `app.Run`).

### Specific Changes

1. **`resolveArgNames`**: Signature changed from `([]string)` to `([]string, error)`
   - Returns `buildProviderNotFoundError` (existing friendly error constructor) when provider not found

2. **`buildInvokeNode`**: Signature changed from `(model.Node)` to `(model.Node, error)`
   - Returns contextual error when closure generation fails

3. **`buildProviderNode`**: Signature changed from `(model.Node)` to `(model.Node, error)`
   - Returns contextual error when closure generation fails

4. **`buildSupplyNode`**: Signature changed from `(model.Node)` to `(model.Node, error)`
   - `printer.Fprint` error is no longer ignored (was `_ = printer.Fprint(...)`)

5. **`buildNodes`**: Signature changed from `([]model.Node)` to `([]model.Node, error)`
   - All sub-function calls now check and propagate errors

6. **`buildFinalNodes`**: Added error propagation
   - Previously called `buildNodes` without checking for errors

## Files Changed

- `internal/extractor/extractor.go`: 6 function signature changes, panics replaced with error returns

## Before / After

**Before** (panic):
```
panic: no provider for type *struct{} with name "missing"

goroutine 1 [running]:
...
github.com/shanjunmei/dig/internal/extractor.(*Extractor).resolveArgNames(...)
    internal/extractor/extractor.go:1315
...
```

**After** (structured error):
```
Package example/gen_failures/missing_provider:
    extract and build nodes: no provider for type *struct{} with name "missing"
        required by dig_invoke_1 (closure) at di.go:17
        (no provider for this type at all)
      💡 Fix: add a provider for *struct{} via dig.Provide or dig.Supply
```

## Backward Compatibility

This is a pure internal refactoring that does not change public APIs or behavior. All error paths are still correctly detected, only converted from panics to structured error returns.
