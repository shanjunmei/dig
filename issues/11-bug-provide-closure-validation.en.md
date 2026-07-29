# Bug: Missing Provide Closure Signature Validation Causes Invalid Code Generation

## Description

During digen code generation, the return signature of `dig.Provide` closures (anonymous functions) was not validated. When users wrote Provide closures with invalid return signatures (e.g., too many return values, second return value not of error type), digen would still generate Go code, but the generated code would fail Go compiler checks.

## Reproduction

### Scenario 1: Provide closure with too many return values

```go
dig.Provide(func() (string, int, error) {
    return "a", 1, nil
}),
```

digen generates:
```go
v0, err := dig_provider_1()  // expects 2 return values, but gets 3
```

Compilation error: `assignment mismatch: 2 variables but dig_provider_1 returns 3 values`

### Scenario 2: Provide closure with non-error second return value

```go
dig.Provide(func() (string, string) {
    return "a", "b"
}),
```

digen generates:
```go
v0 := dig_provider_1()  // expects 1 return value, but gets 2
```

Compilation error: `assignment mismatch: 1 variable but dig_provider_1 returns 2 values`

## Root Cause

In `internal/extractor/extractor.go`, the `validateClosureSignature` function:

- **Invoke closures**: Has `validateInvokeSignature` to validate return signatures ✅
- **Named function Provide**: Validated via `switch res.Len()` in `handleProvide` ✅
- **Provide closures**: **Completely skip return signature validation** ❌

The `sigHasError` function only checks whether the last return value is of error type. For signatures like `(string, int, error)`, it incorrectly treats them as "valid Provide with error return", generating `v0, err := dig_provider_1()` call code.

## Fix

Added return signature validation for Provide closures in `validateClosureSignature`:

```go
// validateProvideSignature validates Provide function return signatures
func validateProvideSignature(sig *types.Signature, funcName string) error {
    res := sig.Results()
    switch res.Len() {
    case 0:
        return fmt.Errorf("func %s has no return", funcName)
    case 1:
        return nil
    case 2:
        if !isErrorType(res.At(1).Type()) {
            return fmt.Errorf("func %s: second return value must be error, got %s", funcName, res.At(1).Type().String())
        }
        return nil
    default:
        return fmt.Errorf("func %s: too many return values (%d), only (T) or (T, error) are allowed", funcName, res.Len())
    }
}
```

And called in the `else` branch (Provide closure) of `validateClosureSignature`.

## Files Changed

- `internal/extractor/extractor.go`: Added `validateProvideSignature` function, modified `validateClosureSignature`

## Behavior After Fix

For the two scenarios above, digen now correctly rejects them **before generating code** and outputs clear error messages:

```
Package example/gen_failures/provide_too_many_returns:
    extract and build nodes: func anonymous provide function: too many return values (3),
        only (T) or (T, error) are allowed
```

```
Package example/gen_failures/provide_bad_error:
    extract and build nodes: func anonymous provide function: second return value must be error, got string
```

## Test Coverage

New test cases under `example/gen_failures/`:
- `provide_no_return`: Provide closure with no return → triggers "has no return"
- `provide_bad_error`: Provide closure with non-error second return → triggers "second return value must be error"
- `provide_too_many_returns`: Provide closure with too many returns → triggers "too many return values (3)"
