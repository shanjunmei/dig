# dig 排错技能（Troubleshooting）

> `system_prompt_dig.md` 的「排错」子模块。生成/构建失败时的排查优先级与常见错误映射。

## 1. 错误格式

dig 所有错误均含源码位置（`file:line:col`）与 `💡 Fix:` 修复建议，无需开启 `-debug` 即可定位。

## 2. 排查优先级

遇到生成或构建失败，按顺序核对：

1. **闭包捕获局部变量**：Provide/Invoke 闭包捕获了 InitApp 局部变量 → 改为包级变量或参数注入
2. **基础类型冲突**：同类型多提供者无区分 → 用包装类型（如 `type UseMySQL bool`）
3. **重复 Module**：同一 Module 被注册多次 → 去重或改名
4. **泛型未实例化**：写了 `dig.Provide(NewStore)` 而非 `dig.Provide(NewStore[int])` → 显式实例化
5. **多实例歧义**：存在同类型多实例但消费者未用参数名区分 → 用命名参数消费
6. **跨包未导出引用**：提升后的闭包内引用了跨包未导出符号 → 改为导出符号或在本包实现

## 3. 常见错误 → 修复

| 现象 | 原因 | 修复 |
|------|------|------|
| `private` / 未导出符号错误 | 闭包内跨包未导出调用 | 导出该符号或在本包实现 |
| 歧义依赖错误 | 同类型多实例未命名区分 | 用命名参数（如 `mainDB`）消费 |
| 变量名遮蔽警告 | 生成代码变量名冲突 | 一般自动由 ShadowGuard 处理；人工检查命名 |
| 重复 InitApp 声明 | di.go 未带 `//go:build digen` 被 `go build` 编译 | 为 di.go 加 build tag |
| provider 声明 context.Context 参数 | 生成期校验拒绝 | 将 context 注入改为在 Invoke 内部使用 |

## 4. 诊断工具

- `digen check [pkgs]`：仅校验 DI 契约（提取 + 未使用提供者检查），不写文件
- `digen graph [pkgs]`：Mermaid 依赖图，定位缺失/环
- `digen explain <type>`：解释某类型如何被解析
- `-debug`：打印别名映射诊断，排查别名相关异常
