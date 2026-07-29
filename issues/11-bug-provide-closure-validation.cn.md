# Bug：Provide 闭包签名校验缺失导致生成无效代码

## 问题描述

在 digen 代码生成过程中，`dig.Provide` 闭包（匿名函数）的返回签名没有经过校验。当用户编写的 Provide 闭包返回了不合法的签名时（如返回值过多、第二返回值非 error 类型），digen 仍然会生成 Go 代码，但生成的代码无法通过 Go 编译器编译。

## 复现场景

### 场景 1：Provide 闭包返回值过多

```go
dig.Provide(func() (string, int, error) {
    return "a", 1, nil
}),
```

digen 生成的代码：
```go
v0, err := dig_provider_1()  // 期望 2 个返回值，但实际返回 3 个
```

编译错误：`assignment mismatch: 2 variables but dig_provider_1 returns 3 values`

### 场景 2：Provide 闭包第二返回值非 error

```go
dig.Provide(func() (string, string) {
    return "a", "b"
}),
```

digen 生成的代码：
```go
v0 := dig_provider_1()  // 期望 1 个返回值，但实际返回 2 个
```

编译错误：`assignment mismatch: 1 variable but dig_provider_1 returns 2 values`

## 根因分析

在 `internal/extractor/extractor.go` 的 `validateClosureSignature` 函数中：

- **Invoke 闭包**：有 `validateInvokeSignature` 校验返回签名 ✅
- **命名函数 Provide**：在 `handleProvide` 中通过 `switch res.Len()` 校验 ✅
- **Provide 闭包**：**完全跳过返回签名校验** ❌

原因是 `sigHasError` 只检查"最后一个返回值是否是 error 类型"，对于 `(string, int, error)` 这样的签名，会错误地当作"合法的带 error 返回的 Provide"，从而生成 `v0, err := dig_provider_1()` 的调用代码。

## 修复方案

在 `validateClosureSignature` 中为 Provide 闭包也添加返回签名校验：

```go
// validateProvideSignature 验证 Provide 函数的返回签名
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

并在 `validateClosureSignature` 的 `else` 分支（Provide 闭包）中调用此校验。

## 涉及文件

- `internal/extractor/extractor.go`：新增 `validateProvideSignature` 函数，修改 `validateClosureSignature`

## 修复后的行为

对于上述两个场景，digen 现在会在**生成代码之前**就正确拒绝并输出清晰的错误信息：

```
Package example/gen_failures/provide_too_many_returns:
    extract and build nodes: func anonymous provide function: too many return values (3),
        only (T) or (T, error) are allowed
```

```
Package example/gen_failures/provide_bad_error:
    extract and build nodes: func anonymous provide function: second return value must be error, got string
```

## 测试覆盖

新增 `example/gen_failures/` 下的测试用例：
- `provide_no_return`：Provide 闭包无返回值 → 触发 "has no return"
- `provide_bad_error`：Provide 闭包第二返回值非 error → 触发 "second return value must be error"
- `provide_too_many_returns`：Provide 闭包返回值过多 → 触发 "too many return values (3)"
