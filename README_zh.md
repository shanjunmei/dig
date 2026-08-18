## LLM 智能助手提示词
优化过的 dig AI 助手提示词统一存放在 [`prompts`](./prompts) 目录，入口为 [`system_prompt_dig.md`](./prompts/system_prompt_dig.md)，覆盖：核心 API 与 CLI、排错、版本迁移、dig/Wire/Fx 对比。

### 官方工业级模块化开发规范手册
一套基于 dig 构建、面向业务微服务的完整标准化生产级编码规范手册：
[工业级模块化编码规范手册](./prompts/industrial_modular_coding_skill_zh.md)

# dig — 编译期依赖注入 for Go

中文文档 | [English](./README.md) | [文档站点](https://shanjunmei.github.io/dig/?lang=zh)

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **当前版本**：v1.0.19 — 完整发版说明见 [CHANGELOG.md](./CHANGELOG.md)。

---

## 为什么选择 dig？

Go 的依赖注入工具分为两大阵营：

- **Uber Fx**：API 优雅（`Provide`/`Invoke`/`Supply`/`Module`），但依赖 **运行时反射** – 启动较慢、依赖错误仅在运行时 panic、二进制体积更大。
- **Google Wire**：编译期安全且零运行时开销，但 **API 冗长且反直觉** – 重复的 `wire.NewSet`、手动接口绑定、`wire.Value` 禁止函数调用与通道接收，以及臭名昭著的 `wire.Build` 必须写 `return nil, nil` 这样的哑占位符。

**dig** 结合了两者优点：**Fx 风格的极简 API** + **Wire 风格代码生成**（无反射、零运行时依赖），外加严格的闭包捕获安全检测、泛型支持、内置 `Invoke`、针对未使用提供者的合理策略，以及**通过参数名原生支持同一类型的多个实例注入**。

> **关于名称的说明**：本项目（`github.com/shanjunmei/dig`）是一个**编译期、代码生成**式的 DI 库，与 Uber 的运行时反射容器 `go.uber.org/dig`（即 `uber-go/fx` 底层的引擎）**毫无关系**。请勿混淆两者。

---

## 核心特性

- **编译期解析** – 在 `go generate` 期间完成依赖图解析，错误在生成阶段即被捕获。
- **零运行时反射与零运行时依赖** – 生成的代码是纯 Go，不导入任何第三方运行时依赖（仅标准库，如 `context`）。
- **极简 API** – 仅需 `Build`、`Provide`、`Supply`、`Invoke`、`Module`。
- **闭包捕获安全** – 内联闭包不能捕获 `InitApp` 中的局部变量，由生成器强制检查。
- **闭包内联**（`-inline`）– 将简单闭包内联为立即调用函数表达式（IIFE），减少生成的函数数量；身份闭包（`func(p T) T { return p }`）塌缩为一次直接类型转换。
- **泛型支持** – 原生支持泛型函数和类型。
- **可观测性** – 支持调试日志，运行时可通过 `Logf` 覆盖；启用 `-debug` 时还会打印每个包解析后的导入别名映射。
- **可操作错误** – 所有错误消息包含源码位置（`file:line:col`）与 `💡 Fix:` 修复建议。
- **未使用提供者策略** – `error`（默认）、`ignore` 或 `drop`。
- **模块嵌套** – 支持层次化组合模块，内置重复检测。
- **命名实例注入** – 通过参数名区分同一类型的多个实例（详见下文）。
- **ShadowGuard** – 生成器级防护，自动检测并避免生成代码中的变量名遮蔽。

---

## 安装

```bash
go get github.com/shanjunmei/dig@latest
go install github.com/shanjunmei/dig/cmd/digen@latest
```
要求 Go 1.25+。

使用 [Mage](https://magefile.org) 构建（可选，自动注入版本信息）：
```bash
mage build    # 构建二进制并从 git 注入版本号
mage install  # 安装到 $GOPATH/bin
mage test     # 运行测试（含竞态检测）
```

> **注意：** 本仓库的 `cmd/digenv1` 是遗留的单文件生成器原型，**不**面向最终用户——请始终使用 `cmd/digen`。

---

## 快速开始

**di.go**（使用构建标签 `//go:build digen`）：
```go
//go:build digen
package main

import (
    "context"
    "github.com/shanjunmei/dig"
)

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out dig_gen.go

func InitApp() func(context.Context) error {
    return dig.Build(
        dig.Provide(NewConfig),
        dig.Provide(NewDB),
        dig.Supply(DefaultTimeout),          // 直接注入值
        dig.Provide(func(t Timeout) *Server { return NewServer(t) }),
        dig.Invoke(func(srv *Server) error { return srv.Run() }),
    )
}
```

**main.go**（业务逻辑）：
```go
package main

import "context"

type Config struct{ Addr string }
func NewConfig() *Config { return &Config{Addr: ":8080"} }

type DB struct{}
func NewDB(*Config) *DB { return &DB{} }

type Timeout int
var DefaultTimeout Timeout = 5

type Server struct{}
func NewServer(Timeout) *Server { return &Server{} }
func (*Server) Run() error { return nil }

func main() {
    if err := InitApp()(context.Background()); err != nil {
        panic(err)
    }
}
```

**生成并运行**：
```bash
digen ./...   # 或 go generate ./...
go run .
```

---

## 核心 API

| 函数 | 用途 |
|------|------|
| `dig.Build(...Option) func(context.Context) error` | 组装容器，返回可执行的函数。 |
| `dig.Provide(any) Option` | 注册构造函数（返回某个值）。 |
| `dig.Supply(any) Option` | 直接注入一个已有值（任意表达式，运行时安全）。 |
| `dig.Invoke(any) Option` | 在所有提供者就绪后执行一个函数（可返回 error）。 |
| `dig.Module(...Option) Option` | 将多个选项组合为可复用、可嵌套的模块。 |

---

## 命名实例注入

dig 支持通过 **参数名** 来区分同一类型的多个实例，适用于以下场景：

- 多个数据库连接（主库、只读库、报表库）
- 多个 Redis 客户端（不同业务域）
- 多个 HTTP 客户端（不同配置）

### 工作原理

1. **定义带有命名返回值的提供者** – 返回值名称成为“实例名称”。
2. **依赖方使用相同的参数名** 来获取特定实例。

### 示例

```go
// 提供者返回两个不同名称的 *sql.DB 实例
dig.Provide(func() (mainDB *sql.DB, reportDB *sql.DB, error) {
    main, err := connectMain()
    if err != nil { return nil, nil, err }
    report, err := connectReport()
    if err != nil { return nil, nil, err }
    return main, report, nil
})

// 使用主库
dig.Invoke(func(mainDB *sql.DB) {
    // mainDB 自动注入
})

// 使用报表库
dig.Invoke(func(reportDB *sql.DB) {
    // reportDB 自动注入
})
```

### 与 `dig.Supply` 配合使用

也可以直接提供命名值：

```go
dig.Supply(mainDB)   // 变量名成为实例名称
dig.Supply(reportDB)
```

生成器使用 **变量名**（而非类型）来区分实例。

### 错误处理

如果同一类型存在多个实例，而消费者未指定参数名，生成器会报错并列出可用名称：

```text
ambiguous dependency: multiple providers for type *sql.DB available:
  - mainDB
  - reportDB
```

### 兼容性

- 原有使用单实例的代码无需改动。
- 该功能为增量添加，无破坏性变更。

---

## 关键约束

### 1. 闭包捕获限制
`Provide`/`Invoke` 中的内联闭包 **不能捕获 `InitApp` 的局部变量** – 仅允许包级符号和字面量（生成器会将闭包提升为包级函数）。  
✅ 允许：`func() Timeout { return DefaultTimeout }`  
❌ 禁止：`t := 5; func() Timeout { return Timeout(t) }`

### 2. 外部参数（InitApp 参数）
`InitApp` 的所有参数会自动注册为 `Supply` 提供者，可在任何地方注入。

### 3. 包装类型解决基本类型冲突
使用不同的类型来避免相同底层类型导致的重复提供者错误（例如多个 `bool`）：
```go
type UseMySQL bool
type UseRedis bool
```

### 4. 泛型
显式实例化泛型类型/函数：
```go
dig.Provide(NewStore[int])
dig.Invoke(Process[string])
```

### 5. 条件逻辑
分支逻辑可在闭包内部（运行时）使用。如需编译期选择，请使用构建标签 – **不要**在 `Module()` 内部放置条件（因为生成器会解析所有分支）。

### 6. 可观测性
运行 `digen -debug` 以注入 `Logf` 调用。运行时覆盖：
```go
var Logf = log.Printf   // 定义在 dig_gen.go 中
func main() { Logf = myLogger.Printf }
```

### 7. 未使用的提供者
`-unused=error|ignore|drop`（默认为 `error`）。

### 8. 包别名策略
`-alias=full|short|obfuscated|numeric` 控制生成的导入别名。启用 `-debug` 时会在生成日志中同时打印每个包解析后的别名映射。

### 9. 闭包内联
`-inline` 将简单的 Provide/Invoke 闭包内联为 IIFE，而不是生成包级命名函数。身份闭包（`func(p T) T { return p }`、`func(p *T) T { return *p }`、`func(p T) *T { return &p }` 以及直接类型转换闭包）会塌缩为单行内联表达式。

### 10. DI 规格文件必须带 `//go:build digen`
每个包含 `dig.Build(...)` 调用的文件**必须**带有 `//go:build digen` 构建标签。digen 在生成的 `dig_gen.go` 上写死 `//go:build !digen`；若源文件不带对应标签，正常 `go build` 会同时编译两个文件并报 `InitApp redeclared`。digen 现在在生成期强制校验——缺标签时直接给出清晰错误与 `💡 Fix:`，而不是让晦涩的重声明错误推迟暴露。

### 11. `context.Context` 仅限 `Invoke` 使用
provider（通过 `dig.Provide` / `dig.Supply` / `dig.Module` 注册的构造函数）**不得**声明 `context.Context` 参数。provider 在 `InitApp` 内被即时（eager）解析，早于运行时 `context.Context` 的产生，故该参数在生成代码中必然 `undefined`。context 注入只对 `dig.Invoke(func(ctx context.Context) { ... })` 合法。digen 在生成期拒绝 provider 侧的 `context.Context` 参数，并给出指向 `dig.Invoke` 或 `dig.Supply` 的 `💡 Fix:`。

### 12. `di.go` 只放接线：类型 / 构造器 / 包级变量必须放在无 `//go:build digen` 的文件或导入包
`di.go`（带 `//go:build digen`）**只能包含 `dig.Build(...)` 接线**（Provide / Invoke / Supply / Module 调用）。所有被接线引用的领域类型、构造函数、包级变量必须定义在**不带该构建标签的文件**（例如 `types.go`）或导入的包里。原因：生成的 `dig_gen.go` 带 `//go:build !digen`，在正常 `go build`（不带 digen 标签）时 `di.go` 被排除，其内符号对生成代码不可见，会导致晦涩的 `undefined: X`。digen 现在在**写文件之前**就做契约预检（`checkContractVisibility`）：一旦接线引用了定义在 digen 文件中的主包符号，立即中止并给出清晰错误与 `💡 Fix:`，而不是事后由类型检查兜底。

> **生成安全网**：生成代码后，digen 会对产出的 `dig_gen.go` 做一次 `go/types` 类型检查。用户源码在加载阶段已通过完整类型检查，因此生成文件上的类型错误只有两类：(a) 真正的**内部生成器 bug**；(b) 漏过契约预检的 **digen 契约违规**（仅发生在 IR 缓存命中、预检被跳过时）。两者都会中止写文件；契约违规给出与预检一致的 `💡 Fix:` 指引，内部 bug 则输出**可点击的预填 GitHub issue 链接**（外加可复制模板）以便上报——绝不会静默产出无法编译的文件。

---

## CLI 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-out` | `dig_gen.go` | 输出文件名（在 `./...` 模式下忽略） |
| `-unused` | `error` | 未使用提供者的处理策略 |
| `-debug` | `false` | 启用调试日志（详细错误始终显示） |
| `-alias` | `full` | 导入别名策略：`full` / `short` / `obfuscated` / `numeric`；启用 `-debug` 时同时打印别名映射诊断 |
| `-inline` | `false` | 将简单闭包内联为 IIFE；身份闭包塌缩为类型转换 |
| `-typecheck` | `true` | 生成后类型检查产出代码以捕获内部生成器 bug；大型 `./...` 运行可关闭（`-typecheck=false`）以省去逐文件重载包图 |
| `-cache` | `false` | 将提取出的 IR 缓存到磁盘，未改动包命中缓存时跳过提取/类型检查 |
| `-cachedir` | `""` | IR 缓存目录（默认：`os.TempDir()/digen-ir-cache`；仅 `-cache` 设置时生效） |
| `-version` | `false` | 打印版本信息并退出 |

---

## CLI 命令

除默认的生成运行（`digen [packages...]`）外，`digen` 还提供用于脚手架、校验与检视的子命令：

| 命令 | 说明 |
|------|------|
| `digen init [path]` | 生成带 `dig.Build` 入口的 `di.go` 脚手架。 |
| `digen check [pkgs]` | 校验 DI 契约（提取 + 未使用提供者检查），**不写**任何文件。 |
| `digen graph [pkgs]` | 以 Mermaid 流程图打印提供者依赖图。 |
| `digen explain <type> [pkgs]` | 解释某类型/提供者如何被解析（按名称或返回类型匹配）。 |
| `digen completion <shell>` | 输出 shell 补全脚本（`bash`、`zsh`、`fish`）。 |

所有标志（`-out`、`-unused`、`-debug`、`-alias`、`-inline`、`-typecheck`、`-cache`、`-cachedir`、`-version`）对生成运行与 `check` / `graph` / `explain` 子命令同样生效。

---

## 对比矩阵

### 架构与方法

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| **方法** | 代码生成 | 代码生成 | 运行时反射 |
| 需要代码生成步骤 | ✅ `digen` | ✅ `wire` CLI | ❌ |
| 零反射 | ✅ | ✅ | ❌ |
| 零运行时依赖 | ✅ | ✅ | ❌（依赖 `fx` + `dig` 运行时） |
| 校验时机 | 生成期 | 生成期 | 运行时（`fx.New` / `fx.ValidateApp`） |
| 提供者初始化 | 即时（调用 `InitApp` 时） | 即时（在生成的注入器中） | 惰性（仅在被消费时执行） |
| 二进制体积影响 | 极小 | 极小 | 中等（`fx` + `dig` + `multierr`） |

### API 设计

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| 核心 API 数量 | 5 个（`Build`/`Provide`/`Supply`/`Invoke`/`Module`） | 7 个（`Build`/`NewSet`/`Value`/`InterfaceValue`/`Bind`/`Struct`/`FieldsOf`） | 15+ 个（`Provide`/`Supply`/`Invoke`/`Module`/`Annotate`/`Annotated`/`Decorate`/`Replace`/`WithLogger`/…） |
| **直接值注入** | ✅ `dig.Supply`（任意表达式） | ⚠️ `wire.Value`（禁止函数调用 / channel 接收） | ✅ `fx.Supply`（仅具体类型；接口需 `fx.As`） |
| 内置 `Invoke` | ✅ | ❌ | ✅ |
| 模块定义方式 | `dig.Module(...Option)` | `var Set = wire.NewSet(...)` | `fx.Module("name", ...)` |
| 模块嵌套 | ✅ 显式支持 | ⚠️ Set 组合（扁平） | ✅ 显式支持，带命名 |
| 模块是否必须命名 | ❌ | N/A | ✅ |
| 模块作用域（私有提供者） | ❌ | ❌ | ✅ `fx.Private` |
| 接口绑定 | 通过身份闭包（如 `func(p *Impl) Iface { return p }`，内联为类型转换） | ✅ 显式 `wire.Bind(new(Iface), new(*Impl))` | ✅ `fx.Annotate(NewImpl, fx.As(new(Iface)))` |
| 结构体字段注入 | ❌ | ✅ `wire.Struct` | ❌（用构造函数） |
| 结构体字段提取 | ❌ | ✅ `wire.FieldsOf` | ❌ |
| **相同类型的多个实例** | ✅ **命名参数** | ❌（需用包装类型） | ✅ **命名 + 值组 (Value Groups)** |
| 值组（同类型集合） | ❌ | ❌ | ✅ `group:"name"`（支持 `flatten` / `soft`） |
| 可选依赖 | ❌ | ❌ | ✅ `optional:"true"` |
| 清理函数 | ❌ | ✅ 第二返回值 `func()`，保证调用顺序 | ✅ 通过 `OnStop` 钩子 |
| 生命周期钩子 (OnStart / OnStop) | ❌ | ❌ | ✅ `fx.Lifecycle` |
| 装饰器（运行时包装 / 替换） | ❌ | ❌ | ✅ `fx.Decorate` / `fx.Replace` |
| 泛型支持 | ✅ 编译期（需显式实例化） | ❌（必须为每个实例化写包装） | ⚠️ 仅已实例化的泛型；无泛型 API |
| 闭包捕获安全 | ✅ 生成器强制检查 | N/A（仅支持函数） | N/A |
| API 友好度 | Fx 风格，极简 | Wire 风格，冗长且反直觉 | Fx 风格，极简 |

### 错误处理与诊断

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| 错误传播模型 | 提供者错误 `panic`（快速失败）；Invoke 错误返回 | 提供者 `error` 返回值，经注入器传播 | `app.Err()`；启动失败回滚 `OnStop` 钩子 |
| 错误含源码位置 | ✅ 每条错误均含 `file:line:col` | ⚠️ 仅提供者 / Set 名称 | ⚠️ 运行时堆栈 |
| 可操作修复建议 | ✅ 每条错误均含 `💡 Fix:` | ❌ | ❌ |
| 未使用提供者策略 | 3 种模式（`error` / `ignore` / `drop`） | 仅硬错误（无模式） | N/A（惰性；静默跳过） |
| 不运行即可校验 | ✅ `digen check` / 生成即校验 | ✅（生成即校验） | ✅ `fx.ValidateApp(opts)` |
| 调试日志 | ✅ 运行时可覆盖 `Logf` | ❌ 手动 | ✅ `fxevent`（Console / Zap / Slog） |
| 依赖图可视化 | ✅ `digen graph`（Mermaid） | ❌ | ✅ `fx.DotGraph` + `fx.VisualizeError`（DOT） |
| 解析路径解释 | ✅ `digen explain <type>` | ❌（阅读生成代码） | ❌（仅运行时错误） |
| 测试辅助 | ❌ | ❌ | ✅ `fxtest` 包 |

### 运行时与运维

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| App 生命周期对象 | ❌（返回裸 `func(ctx) error`） | ❌（返回生成值） | ✅ `*fx.App`（`Start` / `Stop` / `Done` / `Wait`） |
| 信号处理 (SIGINT / SIGTERM) | ❌（调用方负责） | ❌ | ✅ 内置于 `app.Run` |
| 编程式关停 | ❌ | ❌ | ✅ `fx.Shutdowner` + `fx.ExitCode` |
| 可配置启停超时 | N/A | N/A | ✅（默认 15s） |

### 项目状态

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| 维护状态 | ✅ 活跃 | ⚠️ **已归档**（仅修 bug） | ✅ 活跃 |
| 最新版本 | v1.0.19 | v0.7.0（2025 年 8 月，beta） | v1.24.0（2025 年 5 月） |
| Go 版本要求 | 1.25+ | 标准 | 1.21+（用于 `slog` logger） |
| 重构友好度 | 高（静态检查 + 源码位置） | 低（错误晦涩） | 中（运行时错误） |

> **Wire 特别说明**：`wire.Build` 需要写一个哑 `return nil, nil`（或 `panic(wire.Build(...))`）；`wire.Value` 禁止函数调用与 channel 接收（不仅是常量，但接近）；`wire.NewSet` 在分析时被扁平化（无作用域 / 可见性边界）；项目自 v0.7.0 起**已归档**——上游不再接受新功能，但仍接受 bug 修复；不支持泛型（必须为每个实例化编写具体提供者）。
>
> **Fx 特别说明**：功能最丰富——完整的生命周期（`OnStart`/`OnStop` 按依赖顺序执行、逆序销毁）、装饰器（`fx.Decorate`/`fx.Replace`）、支持 `flatten`/`soft` 模式的值组、`fx.Private` 模块作用域、`fxtest` 测试包、`fx.DotGraph` 可视化，以及感知信号的 `app.Run` 与 `fx.Shutdowner`。代价是运行时反射（启动延迟）、依赖错误在运行时 panic（可通过 CI 中的 `fx.ValidateApp` 缓解），以及对 `fx` + `dig` 运行时的硬依赖。
>
> **dig 取舍**：刻意保持极简——无生命周期钩子、无清理函数、无装饰器、无可选依赖、无 App 对象 / 信号处理。`InitApp()` 返回裸 `func(context.Context) error`，优雅关停由调用方负责。作为交换：零运行时开销、编译期安全、含源码位置与 `💡 Fix:` 建议的错误信息、原生泛型支持，以及三者中最小的 API 表面积。

---

## API 快速迁移参考

| 操作 | dig | Wire | Fx |
|------|-----|------|----|
| 构造函数 | `dig.Provide(NewSvc)` | `wire.NewSet(NewSvc)` | `fx.Provide(NewSvc)` |
| 值注入 | `dig.Supply(val)` | `wire.Value(val)`（禁止函数调用） | `fx.Supply(val)` |
| 启动钩子 | `dig.Invoke(fn)` | 不支持 | `fx.Invoke(fn)` |
| 模块分组 | `dig.Module(a, b)` | `wire.NewSet(a, b)` | `fx.Module("name", a, b)` |
| 构建容器 | `dig.Build(...)`（返回可执行函数） | `wire.Build(...)`（哑标记） | `fx.New(...)` |
| 运行 | `run := InitApp(); run(ctx)` | 调用生成的函数 | `app.Run(ctx)` |
| 接口绑定 | `dig.Provide(func(p *Impl) Iface { return p })` | `wire.Bind(new(Iface), new(*Impl))` | `fx.Annotate(NewImpl, fx.As(new(Iface)))` |
| 多实例 | 命名返回值 + 命名参数 | 不支持（包装类型） | `fx.Annotated{Name:"x"}` / `group:"x"` |
| 清理 / 销毁 | 不支持（调用方管理） | 提供者返回 `func()` | `fx.Lifecycle` 的 `OnStop` 钩子 |
| 优雅关停 | 调用方处理信号 | 调用方处理信号 | `app.Run`（SIGINT/SIGTERM）+ `fx.Shutdowner` |

---

## 完整示例

参阅 [`example/`](./example) 目录，包含跨包依赖、泛型、同名模块、嵌套、外部参数、`Supply`、闭包、调试日志、构建标签、别名策略，以及**命名多实例注入**的完整演示。

```bash
cd example
digen -unused=ignore ./...
go run .
```

---

## 许可证

MIT – 参见 [LICENSE](./LICENSE)。
