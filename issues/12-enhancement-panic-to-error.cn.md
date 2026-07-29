# 增强：将 panic 替换为结构化错误返回

## 问题描述

在 `internal/extractor/extractor.go` 中，多处使用 `panic()` 处理错误情况。当遇到非法用户代码时（如循环依赖、缺失提供者等），程序会直接崩溃，输出 Go 运行时 panic 堆栈信息，对用户不友好，且无法被上层捕获和格式化。

## 受影响的位置

在 `internal/extractor/extractor.go` 中，以下位置使用了 `panic`：

| 函数 | 行号 | 触发条件 |
|---|---|---|
| `resolveArgNames` | ~1315 | 找不到参数对应的提供者 |
| `buildInvokeNode` | ~1690 | 生成 Invoke 闭包代码失败 |
| `buildProviderNode` | ~1975 | 生成 Provider 闭包代码失败 |

这些 panic 都是在用户输入非法（如缺失 Provider）或系统异常（如代码生成失败）时触发。

## 改进方案

将所有 `panic` 替换为可传播的 `error` 返回值，使错误能被上层（`buildFinalNodes` → `buildNodes` → `processPackage` → `processor.Process` → `app.Run`）逐级捕获和格式化。

### 具体修改

1. **`resolveArgNames`**：签名从 `([]string)` 改为 `([]string, error)`
   - 找不到提供者时返回 `buildProviderNotFoundError`（已有的友好错误构造函数）
   
2. **`buildInvokeNode`**：签名从 `(model.Node)` 改为 `(model.Node, error)`
   - 闭包生成失败时返回带上下文的 error
   
3. **`buildProviderNode`**：签名从 `(model.Node)` 改为 `(model.Node, error)`
   - 闭包生成失败时返回带上下文的 error

4. **`buildSupplyNode`**：签名从 `(model.Node)` 改为 `(model.Node, error)`
   - `printer.Fprint` 错误不再被忽略（原为 `_ = printer.Fprint(...)`）

5. **`buildNodes`**：签名从 `([]model.Node)` 改为 `([]model.Node, error)`
   - 所有子函数调用现在都会检查并传播 error

6. **`buildFinalNodes`**：添加 error 传播
   - 原代码直接调用 `buildNodes` 不检查返回错误

## 涉及文件

- `internal/extractor/extractor.go`：6 个函数签名变更，panic 替换为 error 返回

## 改进效果

**改进前**：
```
panic: no provider for type *struct{} with name "missing"

goroutine 1 [running]:
...
github.com/shanjunmei/dig/internal/extractor.(*Extractor).resolveArgNames(...)
    internal/extractor/extractor.go:1315
...
```

**改进后**：
```
Package example/gen_failures/missing_provider:
    extract and build nodes: no provider for type *struct{} with name "missing"
        required by dig_invoke_1 (closure) at di.go:17
        (no provider for this type at all)
      💡 Fix: add a provider for *struct{} via dig.Provide or dig.Supply
```

## 向后兼容性

这是纯内部重构，不改变公开 API 或行为。所有错误路径仍然被正确检测，只是从 panic 变为结构化 error 返回。
