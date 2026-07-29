# Enhancement: Always Show Detailed Error Messages for All Failed Packages

## Description

In the previous implementation, when digen processed multiple packages and some failed:
- **Non-debug mode**: Only showed "X package(s) failed", users couldn't see which specific package had what problem
- **Debug mode**: Users needed to add the `-debug` flag to see detailed errors

This made troubleshooting difficult and required users to toggle debug mode repeatedly.

## Solution

### 1. Always Show Detailed Error Messages

Modified the error reporting logic in `internal/app/app.go`:

**Before**:
```go
if failedCount > 0 {
    msg := fmt.Sprintf("%d package(s) with dig.Build found but failed to generate", failedCount)
    if !a.cfg.Debug {
        msg += "\n💡 Run with -debug flag for more detailed error information"
    }
    return errors.New(msg)
}
```

**After**:
```go
var failedErrors []string
for _, pkg := range pkgs {
    if err := a.processor.Process(...); err != nil {
        failedErrors = append(failedErrors, fmt.Sprintf("  Package %s:\n    %s", pkg.PkgPath, err.Error()))
        failedCount++
    }
}

if generatedCount == 0 && failedCount > 0 {
    msg := fmt.Sprintf("%d package(s) with dig.Build found but failed to generate:\n%s", failedCount, strings.Join(failedErrors, "\n"))
    return errors.New(msg)
}
if failedCount > 0 {
    fmt.Printf("[digen] failed packages:\n%s\n", strings.Join(failedErrors, "\n"))
}
```

### 2. Two Failure Scenarios

| Scenario | Behavior |
|---|---|
| **All packages failed** (generatedCount == 0) | Returns error with detailed failure info for all packages |
| **Some packages failed** (generatedCount > 0) | Generation succeeds, but prints detailed failure info to stdout |

### 3. Remove Debug Mode Gating for Error Messages

Previously, debug mode was the only way to see detailed errors. Now:
- Detailed error messages are **always shown**
- Debug mode is only used for `[SUPPLY] before/after`, `[PROVIDE] before/after`, `[INVOKE] before/after` level debug logs

## Files Changed

- `internal/app/app.go`: Error reporting logic in `Run()` method

## Before / After Example

**Before** (non-debug mode):
```
[digen] generated 4/21 packages (17 failed), cost: 1.2s
```
Users don't know which 17 packages failed or why.

**After**:
```
[digen] failed packages:
  Package example/gen_failures/cycle:
    extract and build nodes: circular dependency detected:
    Provide: newA -> *A
    -> Provide: newB -> *B
  💡 Fix: break the cycle by removing or restructuring one of the dependencies
  Package example/gen_failures/missing_provider:
    extract and build nodes: no provider for type *struct{} with name "missing"
    ...
[digen] generated 4/21 packages (17 failed), cost: 1.2s
```

Users can immediately see the specific error reason and fix suggestion for each failed package.

## Backward Compatibility

- The `-debug` CLI flag remains available, but only controls debug log output
- Error message format remains consistent, only the display strategy changed from "on-demand" to "always"
