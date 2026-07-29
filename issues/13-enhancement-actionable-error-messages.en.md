# Enhancement: Add Actionable Fix Suggestions to All Error Messages

## Description

Previously, digen error messages only described "what went wrong" but did not tell users "how to fix it". Users had to spend extra time looking up documentation or guessing solutions.

## Solution

Added `💡 Fix:` suggestions to all error messages, enabling users to directly get troubleshooting directions and solutions from the error output.

## Error Message Improvements

### 1. Loader Layer (`internal/loader/loader.go`)

| Scenario | Before | After |
|---|---|---|
| No dig.Build call | `no function containing dig.Build call found` | `no function containing dig.Build call found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error` |
| Multiple dig.Build functions | `multiple functions containing dig.Build call found (only one allowed)` | `multiple functions containing dig.Build call found (only one allowed)\n  💡 Fix: keep exactly one function with dig.Build per package` |

### 2. Extractor Layer (`internal/extractor/extractor.go`)

| Scenario | Before | After |
|---|---|---|
| Missing dig.Module | `function does not contain dig.Module` | Added `💡 Fix: add a single dig.Module(...) call` |
| dig.Module in control flow | `dig.Module inside control flow` | Added `💡 Fix: pass it as a parameter or move to package level` |
| Multiple dig.Module | `contains multiple dig.Module calls` | Added `💡 Fix: merge all providers/invokes into a single dig.Module call` |
| Cannot resolve function call | `cannot resolve function call` | Added `💡 Fix: define a named function with dig.Module(...)` |
| Circular dependency | `circular dependency detected: A -> B -> A` | Added `💡 Fix: break the cycle by removing or restructuring one of the dependencies` |
| Circular dep (topo sort) | `circular dependency` | `circular dependency detected involving N node(s)` |

### 3. Provider Not Found Errors (`internal/extractor/extractor.go`)

The `buildProviderNotFoundError` function was fully rewritten to generate different fix suggestions based on the scenario:

| Scenario | Fix Suggestion |
|---|---|
| No provider for this type at all | `💡 Fix: add a provider for Type via dig.Provide or dig.Supply` |
| Only one named provider, no default | `💡 Fix: rename parameter to 'name' to match the only named provider, or add a default provider via dig.Provide` |
| Named + default providers exist | `💡 Fix: rename parameter to 'name' to use the named provider, or use a default provider` |
| Multiple named providers | `💡 Fix: rename parameter to one of [name1, name2], or add a default provider via dig.Provide` |
| Requested non-existent name | `💡 Fix: check that the provider with name 'X' exists, or remove the name to use the default provider` |
| Only one named, requested different name | `💡 Fix: rename parameter to 'name' (matches the only named provider), or remove the name to make it default` |

### 4. Processor Layer (`internal/processor/processor.go`)

| Scenario | Before | After |
|---|---|---|
| Unused provider | `unused provider: func (returns Type)` | `unused provider: func (returns Type)\n  💡 Fix: either add an Invoke that consumes Type, or remove this provider; use -unused=ignore to suppress` |

### 5. App Layer (`internal/app/app.go`)

| Scenario | Before | After |
|---|---|---|
| No dig.Build packages | `no packages with dig.Build found` | `no packages with dig.Build found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error` |

## Files Changed

- `internal/loader/loader.go`: 2 error message enhancements
- `internal/extractor/extractor.go`: 8+ error message enhancements, `buildProviderNotFoundError` fully rewritten
- `internal/processor/processor.go`: 1 error message enhancement
- `internal/app/app.go`: 1 error message enhancement

## Before / After Example

**Before**:
```
no provider for type *Service with name "secondary"
```

**After**:
```
no provider for type *Service with name "secondary"
    required by dig_invoke_1 (closure) at example/.../di.go:23
    (available: primary)
  💡 Fix: rename parameter to 'primary' (matches the only named provider),
    or remove the name from the provider's return value to make it default
```
