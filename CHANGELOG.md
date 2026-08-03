# 变更日志

本文件记录 `github.com/shanjunmei/dig` 的所有版本变更。英文版本请参阅 [CHANGELOG_en.md](./CHANGELOG_en.md)。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。



## [v1.0.15] - 2026-08-03

### 修复
- **类型包收集逻辑（核心健壮性修复）**：
  - `collectUsedPkgsFromType` 的 `*types.Named` 分支错误遍历 `TypeParams()`（声明级类型参数，如 `T any`，无真实包路径），改为遍历 `TypeArgs()`（实例化类型实参，如 `*common.Config`），同时保留 `TypeParams()` 遍历以支持约束内跨包引用
  - 不对 `*types.Named` 调用 `walk(t.Underlying())`：自引用类型（如 `type Node struct{ Next *Node }`）会导致无限递归栈溢出；struct 字段/方法签名中的跨包引用由 `collectTypeNameAndUsedPkgs` 的 AST 遍历覆盖
  - 新增 `*types.Signature` 分支：函数/方法签名类型（如 provider 返回 `func(*common.Config) error`、invoke 参数为函数类型）此前不会被递归遍历，导致签名参数/返回值中的跨包引用无法被收集，生成的代码中出现 `func(*github.com/.../common.Config) error` 这种全路径未替换别名，编译失败；现遍历 `Recv`/`Params`/`Results`/`TypeParams` 约束，并依赖 `seen` 去重避免重复
  - `addPkgToUsed` 从「`typePkg` 单值取顶层包」改为直接复用 `collectUsedPkgsFromType` 全树遍历，不再遗漏 Map 键/值、切片元素、泛型实参内嵌的跨包引用
  - `generateClosureDef` 中 `allTypes` 循环同上，闭包参数/自由变量/返回类型不再仅靠 `typePkg` 取顶层包，而是全树收集所有跨包引用并逐个 `EnsureAlias`

### 移除
- **`-debug-aliases` 独立诊断标志**：别名映射诊断输出统一并入 `-debug` 全局日志（`-debug` 开启时自动打印 `[alias]` 前缀的别名信息），不再提供独立 flag
- **`findExcludedPackagesInClosure` 辅助函数**：BFS 传递闭包天然不包含「其它 main 包」和「其它含 dig.Build 的库包」（Go 禁止 import main 包，库包不会被重复 import），额外排除逻辑是死代码，已删除且不影响生成结果
- **每函数仅一个 `dig.Module` 限制**：`findSingleModuleCall` → `findAllModuleCalls`，辅助函数内可有多个 `dig.Module` 调用，其 args 会被合并提取；控制流（if/switch/for/select）内的 Module 调用仍不支持
- 移除 `gen_failures/multi_module` 和 `gen_failures/multi_module_call` 回归用例（不再为错误场景）

### 变更
- **第三方库对比矩阵完善**：README 与系统提示词中的 dig / Wire / Fx 对比表补全架构方法、API 设计、错误处理、运行时运维、项目状态等多维度，并新增各工具的取舍说明
- **`example/setup` 清理**：移除 `full.go` 中冗余的 `digen` 构建标签与仅用于非 digen 构建的 `stub.go` 占位文件

---

## [v1.0.14] - 2026-08-01

### 新增
- **闭包内联（`-inline` 标志）**：将简单 Provide/Invoke 闭包内联为立即调用函数表达式（IIFE），减少生成的函数数量 (#16)
- **身份闭包优化**：`func(p T) T { return p }` 形式的闭包生成直接类型转换，而非包装函数 (#17)
- **四种身份闭包操作场景**：引入 `OpKind` 枚举，重构 `analyzeIdentityClosure` 识别直接返回、取址（`&`）、解引用（`*`）、类型转换四种模式
- **全局日志（`-debug` 标志）统一输出别名诊断**：`-debug` 开启时自动打印 `[alias]` 前缀的包导入别名映射，无需额外 flag
- **失败用例补充**：新增 5 个错误类型的回归用例（`capture_const`、`capture_ctx`、`init_named_return`、`duplicate_provide`、`multi_module_call`）(#18)
- 所有错误消息（extractor/loader/processor）新增源码位置（`file:line:col`）前缀，无需 `-debug` 即可定位到出错的 provider/invoke/闭包
- 修复 `handleProvide` 中 `item.Position` 被 `ConditionalDebugf` 吞掉的 bug（非 debug 模式下 `Position` 始终为空，导致 `checkUnusedProviders` 无法显示位置）

### 修复
- **`writeMainFunc` 硬编码 `"context"`**：忽略用户自定义的 `context` 导入别名（如 `import ctx "context"`），导致生成代码无法编译；现通过 `getPkgAlias` 解析正确别名 (#20)
- **跨包别名泄漏**：`digen ./...` 与 `digen ./<pkg>` 产生不同输出；`LoadImportAliases` 改为通过 BFS 计算可达包闭包，排除其他 main / `dig.Build` 包 (#21)
- **闭包体内 context 参数名未传播**：`writeMainFunc` 修复后，provider/invoke/身份转换代码仍硬编码 `"ctx"`；现已将解析后的参数名贯穿 `buildCallArgs`、`buildIdentityConversion`、`writeProvider`、`writeProviders`、`writeInvokes`，并新增 `pickCtxParamName()` 选择不冲突的回退名 (#22)
- **`-version` 输出哈希不一致**：`digen -version` 通过 `shortCommit()` 截断为 8 字符，而 `mage install` 显示完整 40 字符；已移除 `shortCommit()`，直接输出完整哈希

### 变更
- 不可达错误路径标注为安全网；移除死代码（`checkExportedVisibility`、`checkFreeVarVisibility`、`model.Node.ShortName`）(#19)
- 简化 `findCycle` 签名：`([]int, error)` → `[]int`（回退返回 `nil`）
- 重命名 `BuildExecParams` 为 `buildExecParams`（内部）
- 生成配置默认启用闭包内联优化
- `checkGenerationVisibility`、`findSingleModuleCall`、`ValidateReturnType` 新增 `curPkg`/`fset` 参数，错误消息包含源码位置
- `model.Node` 新增 `Position` 字段，用于 `checkUnusedProviders` 报错
**关闭 #16, #17, #18, #19, #20, #21, #22**

---

## [v1.0.13] - 2026-07-30

### 新增
- **版本信息系统**：新增 `-version` 命令行标志，支持 ldflags 注入与 `git describe` 解析；新增 Mage 构建系统（`mage build/install/test/vet`）
- **Provide 闭包签名校验**：生成代码前校验闭包返回签名（仅允许 `(T)` 或 `(T, error)`），非法签名直接报错而非生成无法编译的代码 (#11)
- **生成失败测试用例**：新增 `gen_failures/` 目录覆盖所有错误路径

### 修复
- **结构化错误替换 panic**：所有错误以结构化 error 返回，包含包名、文件位置和 `💡 Fix:` 修复建议，不再输出 Go 运行时 panic 堆栈 (#12)
- **可操作错误消息**：所有错误信息附带场景化 `💡 Fix:` 修复建议（如缺少 Provider、命名不匹配、循环依赖、未使用 Provider 等）(#13)
- **始终显示详细错误**：失败包的详细错误信息始终显示，不再需要 `-debug` 标志（`-debug` 现仅控制调试日志）(#14)

**关闭 #11, #12, #13, #14**

---

## [v1.0.12] - 2026-07-15

### 修复
- **provider 结果被忽略时错误未检查**：使用空白标识符（`_ = provider()`）时，provider 返回的 error 被静默丢弃，可能导致运行时失败而不 panic

  修复行为：
  - provider 无返回 error：仍用 `_ = fn()`
  - provider 返回 error：生成显式错误检查 `if _, err := fn(args); err != nil { panic(err) }`

  符合 "fail-fast" 原则，与非空白 provider 调用的错误处理一致

---

## [v1.0.11] - 2026-07-10

### 新增
- **命名多实例注入**：通过参数名区分同一类型的多个实例，支持多 DB 连接、多 Redis 客户端等场景

### 修复
- **包名与导入路径不匹配导致编译错误**：生成器错误地使用导入路径最后一段作为包引用名（如 `github.com/redis/go-redis/v9` 用 `v9` 而非 `redis`），导致生成代码出现 `v9.Client` 未定义错误；同时 main 包用户自定义别名（如 `loader "path"`）因别名收集逻辑缺陷而丢失

  根因：`collectPkgAlias` 依赖 `importAliasMap`，但 `loadImportAliases` 未优先保留 main 包别名；无别名时回退到路径最后一段而非实际包名（`pkg.Name`）

  修复：
  - `collectPkgAlias` 先扫描 main 包 AST 获取显式别名
  - 无显式别名时使用包实际默认名（`pkg.Name`）
  - 仅在默认名冲突时调用别名策略生成唯一替代

---

## [v1.0.10] - 2026-07-05

### 修复
- **生成代码编译失败**：
  - 标准库类型（如 `*net/http.ServeMux`）缺少别名替换，导致 `go/format` 解析错误
  - 外部参数（如 `*config.AppConfig`）和闭包签名类型（如 `alias.AliasStrategy`）缺少 import

  根因：
  - `collectPkgAlias` 跳过标准库包（`pkg.Module == nil`），导致 `net/http` 等无别名
  - `populateUsedPkgs` 无条件覆盖 `UsedPkgs`，清除了 `addExternalParams` 已设置的包
  - 闭包生成（`generateClosureDef`）未将类型包路径加入 `usedPkgs`
  - `collectTypeNameAndUsedPkgs` 未处理 `*types.PkgName`，闭包体内包引用（如 `alias.ParseAliasType`）未记录

  修复：
  - 移除 `pkg.Module == nil` 限制，为所有非 main 包生成别名
  - `populateUsedPkgs` 集中处理并保留已设置的值
  - `generateClosureDef` 显式将所有相关类型的包路径加入 `usedPkgs`
  - 恢复 `collectTypeNameAndUsedPkgs` 对 `*types.PkgName` 的处理

---

## [v1.0.9] - 2026-07-01

### 新增
- **`debugCommentf` 辅助函数**：仅在 `-debug` 标志启用时返回格式化字符串，用于生成可选的源码注释，关闭 debug 模式时生成代码更干净

---

## [v1.0.8] - 2026-06-25

### 变更
- **digen v2 提升为主生成器**：
  - `cmd/digenv2` 重命名为 `cmd/digen`，作为当前默认生成器
  - 旧 `cmd/digen`（v1）移至 `cmd/digenv1` 保留向后兼容
  - 更新所有 import 路径和生成指令指向新位置
  - "digen" 成为最新版本的规范名称

---

## [v1.0.7] - 2026-06-20

### 新增
- **泛型参数支持**：
  - `extractedItem` 新增 `GenericArgsStr` 字段，存储清理后的纯泛型类型段 `[T1, T2]`
  - `model.Node` 新增 `GenericArgs` 字段，将泛型信息传递给代码生成器

### 变更
- 重写 `extractGenericArgStr`：仅解析 `IndexExpr`/`IndexListExpr` 索引，去除函数标识符前缀
- 更新生成器渲染逻辑，为 provider/unused-provider/invoke 语句追加泛型参数
- 对每个泛型子类型复用 `replacePkgPathWithAlias`，解决跨包全路径语法问题

---

## [v1.0.6] - 2026-06-15

### 新增
- **函数参数作为隐式 `dig.Supply` 提供者**：包含 `dig.Build` 的函数可接受参数，这些参数自动注册为依赖图中的隐式 Supply 提供者

  关键变更：
  - 新增 `addExternalParams` 将每个函数参数注册为 Supply 节点
  - 生成代码保留原始函数签名
  - 拒绝 `context.Context` 参数（应由调用方通过返回函数的参数传入）
  - 更新 `generateCode`、`writeMainFunc` 等函数包含参数格式化和传播

  示例：
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

### 破坏性变更
- **移除 `*dig.App`**：`InitApp()` 返回 `func(context.Context) error`，生成代码零运行时依赖
- **升级指南**：将 `app.Run(ctx)` 替换为 `run := InitApp(); run(ctx)`

### 变更
- 重构：消除运行时依赖，返回裸闭包
- 重构：移除 `App` 接口定义，清理相关代码
- 新增：支持递归包生成（`digen ./...`）
- 修复：`writeMainFunc` 参数类型
- 回退：移除模块路径中的 `/v2` 后缀

---

## [v1.0.4] - 2026-06-05

### 变更
- **重构：别名策略与函数式工具提取到 `pkg/`**
  - 别名生成逻辑从 `cmd/digen` 移至 `pkg/alias`
  - 策略拆分为独立文件：`alias.go`（接口、short/full）、`alias_obf.go`（混淆）、`alias_numeric.go`（数字）
  - 新增 `pkg/functional`，包含泛型 `Map`/`Reduce`/`Keys` 工具
  - 移除 `cmd/digen` 中未使用的 `alias.go`、`alias_obf.go`、`util.go`

---

## [v1.0.3] - 2026-05-30

### 新增
- **生成闭包的原始包路径注释**：闭包（`__p_*`、`__i_*`）被移动到当前包后，日志中丢失原始包路径；在闭包定义上方添加注释保留此信息，便于调试

---

## [v1.0.2] - 2026-05-25

### 修复
- **drop 模式下未移除未使用的闭包提供者**：使用 `-unused=drop` 时，未使用 provider 的闭包定义仍被输出，导致死代码残留

  修复：
  - 将 flag 变量移至 `main()` 减少包级全局变量
  - 重构 `writeClosureDefs` 接受 `refCount` 和 `unusedMode`，drop 模式下跳过未使用闭包
  - 复用 `refCount` 避免重复计数

  三种 unused 模式（error、ignore、drop）现对普通 provider 和闭包行为一致

---

## [v1.0.1] - 2026-05-20

### 变更
- 代码优化

---

## [v1.0.0] - 2026-05-15

### 新增
- 初始发布
