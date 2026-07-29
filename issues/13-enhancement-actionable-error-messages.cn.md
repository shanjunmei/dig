# 增强：为所有错误信息添加可操作的修复建议

## 问题描述

digen 之前的错误信息只描述了"出了什么问题"，但没有告诉用户"如何修复"。当用户遇到错误时，需要花费额外的时间查找文档或猜测解决方案。

## 改进方案

为所有错误信息添加 `💡 Fix:` 修复建议，使用户能直接从错误信息中获取排查方向和解决方案。

## 涉及的错误信息改进

### 1. Loader 层错误（`internal/loader/loader.go`）

| 错误场景 | 改进前 | 改进后 |
|---|---|---|
| 无 dig.Build 调用 | `no function containing dig.Build call found` | `no function containing dig.Build call found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error` |
| 多个 dig.Build 函数 | `multiple functions containing dig.Build call found (only one allowed)` | `multiple functions containing dig.Build call found (only one allowed)\n  💡 Fix: keep exactly one function with dig.Build per package` |

### 2. Extractor 层错误（`internal/extractor/extractor.go`）

| 错误场景 | 改进前 | 改进后 |
|---|---|---|
| dig.Module 缺失 | `function does not contain dig.Module` | 添加 `💡 Fix: add a single dig.Module(...) call` |
| dig.Module 在控制流内 | `dig.Module inside control flow` | 添加 `💡 Fix: pass it as a parameter or move to package level` |
| 多个 dig.Module | `contains multiple dig.Module calls` | 添加 `💡 Fix: merge all providers/invokes into a single dig.Module call` |
| 无法解析函数调用 | `cannot resolve function call` | 添加 `💡 Fix: define a named function with dig.Module(...)` |
| 循环依赖 | `circular dependency detected: A -> B -> A` | 添加 `💡 Fix: break the cycle by removing or restructuring one of the dependencies` |
| 循环依赖（拓扑排序） | `circular dependency` | `circular dependency detected involving N node(s)` |

### 3. Provider 未找到错误（`internal/extractor/extractor.go`）

`buildProviderNotFoundError` 函数进行了全面重写，根据不同场景生成不同的修复建议：

| 场景 | 修复建议 |
|---|---|
| 完全没有该类型的提供者 | `💡 Fix: add a provider for Type via dig.Provide or dig.Supply` |
| 有唯一命名提供者，无默认 | `💡 Fix: rename parameter to 'name' to match the only named provider, or add a default provider via dig.Provide` |
| 有命名提供者和默认提供者 | `💡 Fix: rename parameter to 'name' to use the named provider, or use a default provider` |
| 有多个命名提供者 | `💡 Fix: rename parameter to one of [name1, name2], or add a default provider via dig.Provide` |
| 请求了不存在的命名 | `💡 Fix: check that the provider with name 'X' exists, or remove the name to use the default provider` |
| 只有一个命名，请求了不同名字 | `💡 Fix: rename parameter to 'name' (matches the only named provider), or remove the name to make it default` |

### 4. Processor 层错误（`internal/processor/processor.go`）

| 错误场景 | 改进前 | 改进后 |
|---|---|---|
| 未使用的 Provider | `unused provider: func (returns Type)` | `unused provider: func (returns Type)\n  💡 Fix: either add an Invoke that consumes Type, or remove this provider; use -unused=ignore to suppress` |

### 5. App 层错误（`internal/app/app.go`）

| 错误场景 | 改进前 | 改进后 |
|---|---|---|
| 无 dig.Build 包 | `no packages with dig.Build found` | `no packages with dig.Build found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error` |

## 涉及文件

- `internal/loader/loader.go`：2 处错误信息增强
- `internal/extractor/extractor.go`：8+ 处错误信息增强，`buildProviderNotFoundError` 全面重写
- `internal/processor/processor.go`：1 处错误信息增强
- `internal/app/app.go`：1 处错误信息增强

## 改进效果示例

**改进前**：
```
no provider for type *Service with name "secondary"
```

**改进后**：
```
no provider for type *Service with name "secondary"
    required by dig_invoke_1 (closure) at example/.../di.go:23
    (available: primary)
  💡 Fix: rename parameter to 'primary' (matches the only named provider),
    or remove the name from the provider's return value to make it default
```
