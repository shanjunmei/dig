# 增强：无条件显示所有失败包的详细错误信息

## 问题描述

在之前的实现中，当 digen 处理多个包时，如果有包生成失败：
- **非 debug 模式**：只显示 "X package(s) failed"，用户无法看到具体是哪个包出了什么问题
- **debug 模式**：用户需要加 `-debug` 参数才能看到详细错误

这导致用户在排查问题时需要反复切换 debug 模式，体验不佳。

## 改进方案

### 1. 始终显示详细错误信息

在 `internal/app/app.go` 中修改错误报告逻辑：

**改进前**：
```go
if failedCount > 0 {
    msg := fmt.Sprintf("%d package(s) with dig.Build found but failed to generate", failedCount)
    if !a.cfg.Debug {
        msg += "\n💡 Run with -debug flag for more detailed error information"
    }
    return errors.New(msg)
}
```

**改进后**：
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

### 2. 两种失败情况的区分

| 场景 | 行为 |
|---|---|
| **所有包都失败**（generatedCount == 0） | 返回错误，包含所有失败包的详细错误信息 |
| **部分包失败**（generatedCount > 0） | 生成成功，但打印所有失败包的详细错误信息到 stdout |

### 3. 移除 debug 模式的错误信息门控

之前 debug 模式是查看详细错误信息的唯一方式。现在改为：
- 详细错误信息**始终显示**
- debug 模式仅用于显示 `[SUPPLY] before/after`、`[PROVIDE] before/after`、`[INVOKE] before/after` 等调试级别的日志

## 涉及文件

- `internal/app/app.go`：`Run()` 方法中的错误报告逻辑

## 改进效果示例

**改进前**（非 debug 模式）：
```
[digen] generated 4/21 packages (17 failed), cost: 1.2s
```
用户不知道哪 17 个包失败了，也不知道为什么。

**改进后**：
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

用户可以立即看到每个失败包的具体错误原因和修复建议。

## 向后兼容性

- 命令行参数 `-debug` 仍然可用，但仅控制调试日志输出
- 错误信息格式保持一致，只是显示策略从"按需显示"变为"始终显示"
