# 变更日志

本文件记录 `github.com/shanjunmei/dig` 的所有版本变更。英文版本请参阅 [CHANGELOG_en.md](./CHANGELOG_en.md)。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [v1.0.20] - 2026-08-21

## 🔧 恒等闭包塌缩与闭包 IIFE 内联解耦

- **恒等闭包塌缩（Phase 4）改为始终应用，不再受 `-inline` 控制**  
  此前恒等闭包（如 `func(p *Impl) Iface { return p }`）与 IIFE 内联被同一个 `-inline` 开关耦合；现恒等闭包无论是否传 `-inline` 都会塌缩为直接类型转换（`T(p)` / `&p` / `*p` / `U(p)`）。恒等闭包是字面等价的零语义变化转换、触发面窄且高频（DI 中大量接口/指针包装），故无条件默认开启。
- **`-inline` 现在仅控制 IIFE 内联（Phase 3），默认仍为 `false`**  
  CLI 帮助文本、`README` 标志表、`docs/index.html`、`prompts/dig-core*.md` 已同步更正此前「`-inline` 同时管恒等」的误导性描述。
- 生成侧 `writeProvider` / `writeInvokes` 始终优先处理 `IsIdentityClosure`，恒等优先于 IIFE 的语义不变。
- **新增类型断言恒等闭包（`OpAssert`）**  
  `func(p any) T { return p.(T) }` 现被识别为恒等闭包并塌缩为内联断言 `p.(T)`（`analyzeIdentityClosure` 新增 `*ast.TypeAssertExpr` 分支，新增 `OpAssert` 枚举）。关键修正：断言类型（如 `ServiceImpl`）可能与返回类型（如 `Service`）不同，生成器现以**断言类型**为准输出 `p.(Impl)` 而非 `p.(Service)`，避免语义改变（原闭包断言到具体类型、断言失败 panic，生成的若断言到接口则会接受任意实现者而偏离原行为）。覆盖 DI 中常见的「窄接口 / any → 具体类型」包装场景；随方案 A 一并常开，不受 `-inline` 影响。

## [v1.0.19] - 2026-08-18

## ✨ 生成期契约预检与诊断增强

- **`di.go` 只放接线的契约预检**  
  新增 `internal/extractor/contract.go` 的 `checkContractVisibility`：在 `BuildFinalNodes` 提取之后、写文件之前，扫描主包中**定义在 `//go:build digen` 文件内**的包级符号（type/func/var/const），并校验每条接线（`dig.Provide` / `Invoke` / `Supply` / 闭包体）是否引用了其中任一符号。命中即中止生成（不写文件）并给出清晰错误与 `💡 Fix:`，指明该符号定义在哪个 digen 文件、应挪到无 tag 文件或导入包。遍历覆盖构造器签名、`dig.Supply` 值类型、闭包签名与自由变量、合并参数类型，并递归穿透指针/切片/映射/通道/泛型实参。配套新增回归示例 `example/gen_failures/contract_digen_symbol/`。

- **生成后类型检查安全网：契约违规与内部 bug 分类**  
  `typeCheckGenerated`（v1.0.18 引入）现对生成文件上的类型错误做分类：若 `undefined: X` 中的 X 是主包定义在 digen 文件中的符号，则作为**契约违规**上报（与预检一致的 `💡 Fix:` 指引）；其余才是**内部生成器 bug**（输出预填 issue 链接）。该分类堵住了 IR 缓存命中（`-cache`）跳过 `BuildFinalNodes`、从而也跳过预检的缺口，是契约违反的最后兜底。  
  ⚠️ 修正了此前的安全网误导措辞：内部 bug 分支不再把"符号定义在 di.go"说成"最可能原因"（若真是如此，早被契约分支捕获），改为明确"这是真正的内部生成器 bug"，仅在用户确实在 digen 文件定义符号时提示迁移。

- **构建约束检查逻辑收敛到共享包**  
  将 `extractor` 与 `generator` 中重复的实现（`fileHasDigenConstraint` / `genFileHasDigenConstraint`、`buildExprRequiresDigen` / `buildExprRequiresDigenGen`）抽取为无内部依赖的叶子包 `internal/buildconstraint`，导出 `FileHasDigenConstraint(f *ast.File) bool` 与 `RequiresDigen(expr string) bool` 作为单一真源；三处调用点（`generator.go`、`extractor/buildtag.go`、`extractor/contract.go`）统一收敛。消除重复逻辑与维护成本，新增 `internal/buildconstraint/buildconstraint_test.go` 单测。

- **黄金文件（golden-file）回归测试**  
  新增 `example/golden/golden_test.go`：运行时发现各 `example/*/dig_gen.go`（含 `//go:build digen` 与 `//go:generate` 指令者），解析其 `//go:generate` 还原精确 flags，重生成到临时 `-out`，`normalize()` 剥离 `//go:generate` 元行后逐字节 diff。任何悄然改变输出（含"能编译但语义漂移"）都会被抓出。覆盖 11 个复杂 example，全部与已提交 golden 逐字节一致；已验证能抓漂移（临时篡改 golden 即 FAIL）。

- **`digen` CLI 子命令**  
  新增 `init`（生成带 `dig.Build` 入口的 `di.go` 脚手架）、`check`（仅校验 DI 契约、不写文件）、`graph`（以 Mermaid 打印提供者依赖图）、`explain <type>`（解释类型/提供者的解析路径）、`completion <shell>`（bash/zsh/fish 补全脚本）。`init`/`check`/`graph`/`explain` 复用生成管线，所有 flag 对其同样生效；配套的 `digen -h` 帮助文本与补全脚本同步列出全部 flag 与子命令。

---

## [v1.0.18] - 2026-08-15

## ✨ 生成期强化与诊断

- **禁止 provider 声明 `context.Context` 参数**  
  新增 `checkProviderContextParams`：provider（构造函数，无论经 `Provide`、闭包还是 `dig.Module` 包裹）一旦声明 `context.Context` 参数，在生成期直接报错并给出 `file:line:col` 与 `💡 Fix:`，中止生成、不写坏文件。  
  原因：provider 在 `InitApp` 内被即时（eager）解析，早于运行时 `context.Context` 的产生，故该参数在生成代码中必然 `undefined`；context 注入只对 `dig.Invoke`（运行在内层 `func(ctx)` 里）合法。配套新增回归示例 `example/gen_failures/provider_ctx/`。

- **生成期强制校验 `//go:build digen` 构建标签**  
  新增 `checkBuildSourceConstraint`：含 `dig.Build` 调用的源文件若缺少 `//go:build digen` 约束，在 `BuildFinalNodes` 阶段报错并给出 `💡 Fix:`，中止生成（不写文件）。原因：生成文件由 digen 写死 `//go:build !digen`，若源文件不带对应标签，正常 `go build` 时两者都会定义 `InitApp` 导致 `redeclared`。把这条"铁的原则"从文档约定升级为生成器真守住的不变量。

- **生成后 `go/types` 类型检查安全网**  
  `internal/generator/generator.go` 新增 `typeCheckGenerated`：在 `format.Source` 之后、`os.WriteFile` 之前，对生成的 `dig_gen.go` 做一次 `go/types` 类型检查。由于用户源码在加载阶段（`packages.Load` + `NeedTypes` + `-tags=digen`，且 `dig_gen.go` 因 `//go:build !digen` 被排除）已做过完整类型检查，生成文件上的任何类型错误**无条件等价于内部生成器 bug**。触发时：① 不写出坏文件；② 错误显式标注"内部生成器 bug"，附生成文件位置；③ 输出**可点击的预填 GitHub issue 链接**（`https://github.com/shanjunmei/dig/issues/new?title=...&body=...`）与一份可复制的 issue 模板，引导上报。基础设施失败则 best-effort 回退为正常写文件。该网只兜底未知的提取/生成缺陷，绝不替代语义层规则、也绝不伪装成用户错误。

- **`dig.Supply` 的模块参数不再生成跨包未定义引用**  
  修复生成器缺陷：当 `dig.Supply(x)` 的 `x` 是被内联 `dig.Module` 的函数参数或局部变量时，生成代码会把它限定为包符号（如 `user.cfg`），触发生成后安全网的 `undefined: <pkg>.<param>`。此类自由变量由目标函数自身作用域捕获，现改为原样引用；只有包级符号（var/func/const/type）仍保留限定，对 `db.Index`、`role.Config("production")` 等既有用例行为不变。该修复同时避免内联后的 supply 为源包引入"已导入但未使用"的包。新增回归示例 `example/supply_param/`（+ `example/supply_param_helper/`），由 `example/successtest` 自动发现。

- **用 AST 精确改写取代正则改写类型名**  
  废弃脆弱的 `replaceTypeNames` 正则字符串改写（word-boundary 正则会误改写字符串字面量 / 注释内的同名 token，且按裸名匹配可能误伤同名局部变量），改用基于 `go/ast` + `astutil.Apply` 的精确改写：先以 `token.Pos` 为 key 收集 `Pos -> "alias.Name"` 重写计划（显式跳过 `SelectorExpr.Sel`），在克隆体上对命中的标识符做替换，绝不触碰字符串字面量、注释或同名局部变量。类型字符串路径 → 别名的 `replacePkgPathWithAlias` 保留以向后兼容。生成产物行为零变化。

- **稳定可序列化 IR 与可选磁盘缓存**  
  将 extractor→generator 的中间表示 `[]model.Node` 正式化为可序列化、带 schema 版本的稳定 IR：`internal/model` 新增 `CachedExtraction`（Nodes + ImportAliasMap/PkgAliasMap/PkgNameMap + `SchemaVersion`），`Node` / `Arg` 补全 JSON tag；`UnmarshalJSON` 在 `SchemaVer` 不匹配时直接报错而非静默误读。新增 `internal/ir` 包负责磁盘读写（默认 JSON、原子写），并由 `cmd/digen` 的 `-cache`（默认关）/ `-cachedir` 开关启用。开启后未改动的包可跳过昂贵的提取 / 类型检查步骤直接复用缓存；cache key 覆盖配置旋钮、`runtime.Version()` 与（递归）本包及传递依赖的源文件内容哈希，依赖 API 变化会自动使缓存失效，无需手动清理。缓存路径任何失败都优雅回退到重新提取，且默认关闭不影响生成语义。

## 🐛 修复 v1.0.17 引入的回归

- **闭包参数 / 局部变量被误报为 `private`（"var X is private"）**  
  v1.0.17 新增的 `checkFunctionVisibilityInClosure` 在生成前遍历闭包体内所有裸标识符调用（`fn(args)` / `T(x)`）并对函数标识符调用 `checkGenerationVisibility`。但 `checkGenerationVisibility` 会把闭包参数（及局部函数 / 类型变量）这类 `*types.Var` / `*types.TypeName` 也按"包级未导出符号"处理：当闭包定义在**不同于生成目标**的包中（典型如跨包模块内联 `user.Module(cfg)`，闭包在 `user` 包、生成目标在 `cmd/app`），`curPkg != mainPkgPath` 且其名未导出，于是被误判为"跨包不可见"而报错，导致**合法的闭包参数调用反而无法生成**。`dig.Invoke(func(f func() Config){ _ = f() })` 即会触发 `var "f" is private`。  
  修复（`internal/extractor/visibility.go`，提交 `81e2a78`）：在 `checkGenerationVisibility` 的 `*types.Var` 分支加 `if !isPackageLevelVar(o) { return nil }`，只对真正的包级变量做可见性校验；`*types.TypeName` 同类也按 `isPackageLevelVar` 思路防护。闭包体本就会内联进目标包，参数与局部变量根本不构成跨包引用，故一律放行。该改动同时消除对 `private_visibility` / `closure_private_fn` 之外合法用例的误伤。

## 📦 示例与文档更新

- **`gen_failures` 自动化回归测试**  
  新增 `example/gen_failures/gentest/gen_failures_test.go`：遍历各子目录、自行构建 digen（临时路径）、断言非零退出 + 命中预期错误子串，让 `provider_ctx` 等失败样例被 CI 自动兜住。另给 4 个原本缺标签的 fixture（`ambiguous`/`cycle`/`missing_provider`/`named_mismatch`）补了 `//go:build digen`（仅加标签，不动逻辑）。
- 修正 `prompts/system_prompt_dig.md` / `_en.md` 中"digen 静态校验"的不实宣称（源文件 `//go:build digen` 现由 digen 在生成期校验）。

---

## [v1.0.17] - 2026-08-13

## 🐛 Bug 修复

- **修复闭包内裸函数/类型调用未拦截跨包未导出符号**  
  提取器在把闭包从原包提升到生成目标包时，会对裸标识符调用补上包前缀。若原包中存在未导出函数（如 `buildAuditAuthorizer`），提升后变成非法的 `pkg.buildAuditAuthorizer` 跨包引用；而 `digen` 不在生成后编译产物，会静默 `exit 0` 产出无法编译的代码。

  修复：
  - 新增 `checkFunctionVisibilityInClosure`：生成前遍历闭包体内所有裸标识符调用（`fn(args)` / `T(x)`），通过 `checkGenerationVisibility` 拦截跨包未导出符号（同包 / 导出 / 内建符号放行），直接给出清晰的 `private` 错误且不产出坏文件
  - 扩展 `checkGenerationVisibility` 的 switch 增加 `*types.TypeName` 分支，顺带覆盖类型转换 `T(x)` 中未导出跨包类型
  - 新增回归示例 `example/gen_failures/closure_private_fn/`

- **修复示例 `example/db/db.go` 中 `RedisClient.Ping` 参数遮蔽**  
  `Ping(index RedisDbIndex)` 内部 `fmt.Printf` 误用包级变量 `Index` 而非参数 `index`，导致参数形同虚设。已改为正确引用参数 `index`。

- **修复 Supply 重复绑定默认实例错误消息**  
  两个默认（未命名）`dig.Supply` 提供同一类型时，错误消息错显 `with name ""` 而非 `(default)`。根因是未命名场景下 `keyNamed == keyDefault` 导致命名键检查先命中；已重构为 `if instanceName != "" { ... } else { ... }` 分支，默认实例错误信息正确显示 `(default)`。

## ♻️ 重构与优化

- **生成器配置与示例调整**  
  重新生成所有示例的 `dig_gen.go`，统一 `dv` 变量前缀、默认开启 inline 内联模式、移除生成代码中的调试日志输出。
- **未使用 provider 示例改造**  
  `example/gen_failures/unused_provider` 改用 `dig.Provide` + `dig.Supply` 模式注册 provider。

---

## 📦 示例与文档更新

- **新增成功示例**  
  `example/app_runtime_err`（运行时错误传播路径：provider 返回 error 触发 panic、invoke 返回 error 向上传播）、`example/app_xpkg_generic`（跨包泛型 `cache.Cache[*common.Config]`，泛型类型与类型实参均来自不同包）。
- **新增失败示例**  
  `example/gen_failures/` 下新增 `ambiguous`、`duplicate_named`、`duplicate_supply`、`private_visibility`、`unused_provider`，覆盖命名实例歧义、重复命名绑定、重复默认 Supply、跨包未导出可见性、未使用 provider 等错误路径。
- **GitHub Pages 站点**  
  新增 `docs/` 静态站点（项目介绍、核心特性、快速开始、API 概览、对比矩阵、示例、版本时间线），并添加 `.nojekyll`。
- **GitHub Pages 中 / 英文切换**  
  站点新增 中文 / EN 切换（localStorage 持久化、同步标题与 meta、代码块保持语言中立）。
- **系统提示词与工业级编码规范文档更新**  
  精简并同步 `prompts/` 下文档。

---

## 🔧 杂务

- 同步更新 README / CHANGELOG / 系统提示词中的版本号至 v1.0.17。

---

## [v1.0.16] - 2026-08-07

## ✨ 新功能

- **ShadowGuard 变量名遮蔽保护**  
  新增 shadow protection 机制，在代码生成阶段自动检测并避免变量名遮蔽问题，提升生成代码的健壮性。

- **未使用参数默认行为调整**  
  将未使用参数的处理从默认 `drop`（丢弃）改为 `error`（报错），帮助开发者更早发现潜在的参数遗漏问题。

## 🐛 Bug 修复

- **修复泛型类型参数日志引号缺失**  
  完善生成器在打印 Identity Target Type 时类型参数的引号包裹，使日志输出格式更统一、更易读。

- **修复跨包裸函数调用标识符处理**  
  修复提取器在处理跨包裸函数调用时标识符识别不正确的问题。

- **修复自由变量遮蔽包别名问题**  
  解决自由变量参数名遮蔽非闭包 item 中包别名的问题。调整提取逻辑，提前注册包别名，确保 ShadowGuard 在构建参数列表和自由变量映射时能正确处理。

- **修复变量名大小写导致的遮蔽问题**  
  将包级变量 `Db` 重命名为 `db`，避免与导入的 `db` 包别名产生命名遮蔽，并同步更新所有代码引用。

## ♻️ 重构与优化

- **重构 ShadowGuard 与自由变量处理**  
  重构自由变量遮蔽处理逻辑，完善 ShadowGuard 机制，增强代码生成器的健壮性。

- **生成代码元数据增强**  
  在生成的 `dig_gen.go` 文件中添加调试日志和更清晰的元数据信息，便于问题排查。

---

## 📦 示例与文档更新

- **更新 Redis DB Index 配置示例**  
  在 `example` 模块中新增 `RedisDbIndex` 类型及默认变量，更新 `Ping` 方法以接收并记录 db index，通过 DI 注入 db index。

- **更新所有生成的 dig_gen.go 文件**  
  同步更新各示例模块中重新生成的 `dig_gen.go` 文件。

---

## 🔧 杂务

- 依赖安装版本及版本变更日志更新
- 移除 `debug-aliases` 标志，解除“单函数单模块”限制
- 完善第三方库对比矩阵，清理示例代码

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
