## LLM 智能助手提示词
所有大模型专用系统指令文件统一存放在 [`prompts`](./prompts) 目录
- [`system_prompt_dig.md`](./prompts/system_prompt_dig.md)：适配 github.com/shanjunmei/dig 编译期DI库的专业AI开发技能

### 官方工业级模块化开发规范手册
一套基于 dig 构建、面向业务微服务的完整标准化生产级编码规范手册：
[工业级模块化编码规范手册](./prompts/industrial_modular_coding_skill_zh.md)

# dig — 编译期依赖注入 for Go

中文文档| [English](./README.md) 

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **版本**：v1.0.5 – `InitApp()` 返回 `func(context.Context) error`；生成的代码对 `dig` 包 **零运行时依赖**。  
> **从 v1.0.4 升级**：将 `app.Run(ctx)` 替换为 `run := InitApp(); run(ctx)`。

---

## 为什么选择 dig？

Go 的依赖注入工具分为两大阵营：

- **Uber Fx**：API 优雅（`Provide`/`Invoke`/`Supply`/`Module`），但依赖 **运行时反射** – 启动较慢、依赖错误仅在运行时 panic、二进制体积更大。
- **Google Wire**：编译期安全且零运行时开销，但 **API 冗长且反直觉** – 重复的 `wire.NewSet`、手动接口绑定、`wire.Value` 仅限于编译期常量，以及臭名昭著的 `wire.Build` 必须写 `return nil, nil` 这样的哑占位符。

**dig** 结合了两者优点：**Fx 风格的极简 API** + **Wire 风格代码生成**（无反射、零运行时依赖），外加严格的闭包捕获安全检测、泛型支持、内置 `Invoke`，以及针对未使用提供者的合理策略。

---

## 核心特性

- **编译期解析** – 在 `go generate` 期间完成依赖图解析，错误在生成阶段即被捕获。
- **零运行时反射与零运行时依赖** – 生成的代码是纯 Go，不导入任何额外包。
- **极简 API** – 仅需 `Build`、`Provide`、`Supply`、`Invoke`、`Module`。
- **闭包捕获安全** – 内联闭包不能捕获 `InitApp` 中的局部变量，由生成器强制检查。
- **泛型支持** – 原生支持泛型函数和类型。
- **可观测性** – 支持调试日志，运行时可通过 `Logf` 覆盖。
- **未使用提供者策略** – `error`（默认）、`ignore` 或 `drop`。
- **模块嵌套** – 支持层次化组合模块，内置重复检测。

---

## 安装

```bash
go get github.com/shanjunmei/dig@v1.0.8
go install github.com/shanjunmei/dig/cmd/digen@latest
```
要求 Go 1.21+。

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

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out di_gen.go

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
var Logf = log.Printf   // 定义在 di_gen.go 中
func main() { Logf = myLogger.Printf }
```

### 7. 未使用的提供者
`-unused=error|ignore|drop`（默认为 `error`）。

### 8. 包别名策略
`-alias=full|short|obfuscated` 控制生成的导入别名。

---

## CLI 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-out` | `di_gen.go` | 输出文件名（在 `./...` 模式下忽略） |
| `-unused` | `error` | 未使用提供者的处理策略 |
| `-debug` | `false` | 启用调试日志 |
| `-alias` | `full` | 导入别名策略 |

---

## 对比矩阵

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| **方法** | 代码生成 | 代码生成 | 运行时反射 |
| 零反射 | ✅ | ✅ | ❌ |
| 零运行时依赖 | ✅ | ✅ | ❌（需要 fx 运行时） |
| 验证时机 | 生成时 | 生成时 | 运行时（panic） |
| **直接值注入** | ✅ `dig.Supply`（任意表达式） | ⚠️ `wire.Value`（仅常量，繁琐） | ✅ `fx.Supply` |
| 闭包捕获安全 | ✅ 强制检查 | ❌（静默出错） | N/A |
| 内置 `Invoke` | ✅ | ❌ | ✅ |
| 模块定义方式 | `func Module() dig.Option` | `var Set = wire.NewSet(...)` | `fx.Module("name", ...)` |
| 模块嵌套 | ✅ 显式支持 | ⚠️ Set 组合（扁平） | ✅ 显式支持，带命名 |
| 泛型支持 | ✅ 编译期 | ⚠️ 显式且繁琐 | ✅ 反射 |
| 未使用提供者策略 | 3 种模式 | 仅 `drop` | N/A |
| 调试日志 | ✅（运行时覆盖） | ❌ 手动 | ⚠️ 跟踪（非调试） |
| API 友好度 | Fx 风格，极简 | Wire 风格，冗长且反直觉 | Fx 风格，极简 |
| 重构友好度 | 高（静态检查） | 低（晦涩错误） | 中（运行时错误） |

> **Wire 特别说明**：`wire.Build` 需要写一个哑 `return nil, nil`；`wire.Value` 仅支持常量；`wire.NewSet` 的组合是扁平的，非嵌套。

---

## API 快速迁移参考

| 操作 | dig | Wire | Fx |
|------|-----|------|----|
| 构造函数 | `dig.Provide(NewSvc)` | `wire.NewSet(NewSvc)` | `fx.Provide(NewSvc)` |
| 值注入 | `dig.Supply(val)` | `wire.Value(val)`（仅常量） | `fx.Supply(val)` |
| 启动钩子 | `dig.Invoke(fn)` | 不支持 | `fx.Invoke(fn)` |
| 模块分组 | `dig.Module(a, b)` | `wire.NewSet(a, b)` | `fx.Module("name", a, b)` |
| 构建容器 | `dig.Build(...)`（返回可执行函数） | `wire.Build(...)`（哑标记） | `fx.New(...)` |
| 运行 | `run := InitApp(); run(ctx)` | 调用生成的函数 | `app.Run(ctx)` |

---

## 完整示例

参阅 [`example/`](./example) 目录，包含跨包依赖、泛型、同名模块、嵌套、外部参数、`Supply`、闭包、调试日志、构建标签和别名策略的完整演示。

```bash
cd example
digen -unused=ignore ./...
go run .
```

---

## 许可证

MIT – 参见 [LICENSE](./LICENSE)。
