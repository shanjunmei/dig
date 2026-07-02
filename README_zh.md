# dig — Go 编译时依赖注入

[中文文档](./README_zh.md) | English

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **📢 版本说明**
> - **v1.0.4** – 最后一个包含 `*dig.App` 结构的版本
> - **v1.0.5** – `InitApp()` 直接返回 `func(context.Context) error`，无运行时依赖
> 迁移指南请见 [从 v1.0.4 升级](#从-v104-升级)。

**dig** 是一个基于代码生成的 Go 依赖注入容器。  
它在**编译时**解析所有依赖，并生成纯 Go 源代码 —— **无反射**、无运行时魔法，只有静态的、原生的 Go 代码。

---

## 为什么选择 dig？

- **零反射** – 启动性能等同于手写初始化代码。
- **静态安全** – 缺失或循环依赖在 `go generate` 阶段就被捕获，而非运行时。
- **极简 API** – 仅有 4 个核心函数：`Build`、`Provide`、`Supply`、`Invoke`。
- **内置 `Supply`** – 直接注入包级全局变量，无需冗长的包装器。
- **包装类型** – 通过轻量级类型别名解决基础类型注入冲突。
- **内置 `Invoke`** – 所有启动/注册逻辑在所有提供者就绪后执行。
- **可观测性** – 可选的调试日志，带 `before/after` 标记；`Logf` 可在运行时覆盖。
- **未使用提供者策略** – 可选择 `error`（默认）、`ignore` 或 `drop`。
- **零运行时依赖** – 生成的代码在运行时不再导入 `dig` 包。
- **小体积二进制** – 无嵌入式反射或框架运行时。
- **低学习曲线** – 仅 4 个 API 和少量清晰约束。

---

## 安装

```bash
go get github.com/shanjunmei/dig@v1.0.5
go install github.com/shanjunmei/dig/cmd/digen@latest
```

需要 Go 1.21+。

---

## 1. 基本用法

使用 **dig** 的最简方式涉及两个文件。

### 1.1 容器定义（`di.go`）

该文件是**唯一**需要你手动编写的依赖注入装配文件。它使用 `//go:build digen` 构建标签，以便仅在代码生成时被解析。

```go
//go:build digen
package main

import (
    "context"
    "fmt"
    "github.com/shanjunmei/dig"
)

//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -out di_gen.go

func InitApp() func(context.Context) error {
    return dig.Build(
        // 1) 普通函数构造器
        dig.Provide(NewConfig),
        dig.Provide(NewDB),

        // 2) 直接提供一个值（适用于常量或全局变量）
        dig.Supply(DefaultTimeout),

        // 3) 使用内联闭包提供 —— 只要它只使用常量字面量或包级变量即可
        dig.Provide(func(timeout Timeout) *Server {
            return NewServer(timeout)
        }),

        // 4) Invoke 在所有提供者就绪后执行。
        //    它可以返回 error，该错误将被传播给调用方。
        dig.Invoke(func(srv *Server) error {
            return srv.Run()
        }),
    )
}
```

### 1.2 业务逻辑（`main.go`）

此文件包含所有类型、构造器和 `main()` 函数。它**不带**构建标签。

```go
package main

import (
    "context"
    "fmt"
    "time"
)

// ---------- Config ----------
type Config struct {
    Addr string
}

func NewConfig() (*Config, error) {
    return &Config{Addr: ":8080"}, nil
}

// ---------- DB ----------
type DB struct {
    cfg *Config
}

func NewDB(cfg *Config) (*DB, error) {
    if cfg == nil {
        return nil, fmt.Errorf("config cannot be nil")
    }
    return &DB{cfg: cfg}, nil
}

func (db *DB) Ping() error {
    fmt.Println("DB ping OK")
    return nil
}

// ---------- Timeout（包装类型） ----------
type Timeout time.Duration

var DefaultTimeout = Timeout(5 * time.Second)

// ---------- Server ----------
type Server struct {
    db      *DB
    timeout Timeout
}

func NewServer(timeout Timeout) *Server {
    return &Server{timeout: timeout}
}

func (s *Server) Run() error {
    fmt.Printf("Server running with timeout %v\n", s.timeout)
    return nil
}

// ---------- Main ----------
func main() {
    run := InitApp()
    if err := run(context.Background()); err != nil {
        fmt.Printf("app failed: %v\n", err)
    }
}
```

### 1.3 生成并运行

有两种方式调用生成器：

```bash
# 仅生成当前包（默认）
digen

# 递归生成所有子包
digen ./...
```

或者通过 `go generate`（如果你的 `di.go` 中包含了 `//go:generate` 指令）：

```bash
go generate ./...
```

生成后，构建并运行你的应用程序：

```bash
go run .
```

生成器会在每个包含 `dig.Build` 调用的包目录下输出 `dig_gen.go`。每个生成的文件都是自包含的，运行时不再依赖 `dig` 包。

---

## 2. 高级用法

### 2.1 Supply – 注入已存在的值

`Supply` 直接注入一个值，无需构造器函数。它非常适合常量、包级变量或运行时计算的值。

```go
dig.Supply(globalDBConn)
dig.Supply(apiKey)
dig.Supply(DefaultTimeout)
```

### 2.2 包装类型解决基础类型冲突

如果需要多个 `bool`、`string` 或 `time.Duration` 值，请将它们包装为不同的类型，以避免重复提供错误。

```go
// main.go
package main

type UseMySQL bool
type UseRedis bool
type QueryTimeout time.Duration
type DialTimeout time.Duration
```

在 `di.go` 中：
```go
dig.Provide(func() UseMySQL { return UseMySQL(true) }),
dig.Provide(func() UseRedis { return UseRedis(false) }),
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) }),
dig.Provide(func() DialTimeout { return DialTimeout(2 * time.Second) }),
```

生成器将每个包装类型视为唯一，因此不会冲突。

### 2.3 闭包约束（关键）

当你在 `dig.Provide` 或 `dig.Invoke` 内部编写内联匿名函数时，**绝不能**捕获 `InitApp()` 外部作用域中的变量。

✅ **正确** – 仅使用常量字面量或包级变量：
```go
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) })
dig.Provide(func() Timeout { return DefaultTimeout }) // DefaultTimeout 是包级变量
```

❌ **错误** – 捕获了局部变量 `t`：
```go
t := 5 * time.Second
dig.Provide(func() QueryTimeout { return QueryTimeout(t) })
// 生成器报错：无法捕获局部变量 "t"
```

**为什么？** 生成器会提取闭包体并将其提升为 `di_gen.go` 中的顶层函数。这个新函数定义在**包级别**，它无法访问 `InitApp()` 的栈帧。只有包级符号（全局变量、常量、函数、类型）和字面量值可以访问。

**那 `InitApp` 的参数呢？** 它们也是 `InitApp` 的局部变量，同样不能被捕获。然而，`digen` 会自动将所有 `InitApp` 参数注册为 `Supply` 提供者，所以你无需捕获它们 —— 只需在 `Provide`/`Invoke` 闭包中直接请求这些类型即可。

### 2.4 外部参数（InitApp 参数）

你可以向 `InitApp` 传递外部依赖作为参数。生成器会自动将每个参数注册为 `Supply` 提供者，使其可供注入。

```go
// di.go
//go:build digen
func InitApp(cfg *Config, logger *Logger) func(context.Context) error {
    return dig.Build(
        dig.Provide(NewDB),          // 可依赖 *Config（来自参数）
        dig.Invoke(func(db *DB) error { // 也可依赖 *Config 或 *Logger
            logger.Info("db initialized")
            return nil
        }),
    )
}
```

**工作原理**：生成器为 `InitApp` 的每个参数添加一个 `Supply`，因此你可以直接在提供者和调用中使用这些类型。它们与显式 `dig.Supply` 调用同等对待。

**关键区别**：
- **`InitApp` 参数** 被自动作为供应注入。
- **`InitApp` 内部定义的局部变量** 不能被闭包捕获。

如果你需要在 `InitApp` 内部提供一个运行时计算的值，请将其定义为包级变量或函数，或使用 `dig.Supply` 配合包级变量。

### 2.5 泛型支持

`digen` 完整支持泛型函数和类型。你可以直接在 `Provide` 和 `Invoke` 中使用它们。

**泛型类型**：
```go
type Store[T any] struct { items []T }
func NewStore[T any]() *Store[T] { return &Store[T]{} }
```

在 `di.go` 中：
```go
dig.Provide(NewStore[int])          // 具体实例化
dig.Provide(NewStore[string])       // 另一个具体实例化
dig.Invoke(func(s *Store[int]) error { ... })
```

**泛型函数**：
```go
func Process[T any](s *Store[T]) error { ... }
```

```go
dig.Invoke(Process[int])            // 实例化泛型函数
dig.Invoke(Process[string])
```

**泛型闭包**：
```go
dig.Provide(func() *Store[bool] { return NewStore[bool]() })
```

生成器会正确处理泛型实例化，并在生成的函数调用中包含类型参数。

### 2.6 条件逻辑

由于 dig 是**代码生成器**而非运行时框架，条件逻辑在 `Provide`/`Invoke` 闭包内部**可以正常工作**，但在 `Module()` 函数内部则不行。

✅ **闭包内部 —— 运行时生效**：
```go
dig.Provide(func(useMySQL UseMySQL) Store {
    if useMySQL {
        return NewMySQLStore()
    }
    return NewRedisStore()
})
```

❌ **`Module()` 内部 —— 不会按预期工作**（所有分支都会被解析并注册）：
```go
func Module() dig.Option {
    if enableCache {
        return dig.Module(dig.Provide(NewCache))
    }
    return dig.Module(dig.Provide(NewNoop))
}
// NewCache 和 NewNoop 都会被注册
```

**使用构建标签实现编译时选择**：
```go
// module_cache.go
//go:build enable_cache
func Module() dig.Option { return dig.Module(dig.Provide(NewCache)) }

// module_noop.go
//go:build !enable_cache
func Module() dig.Option { return dig.Module(dig.Provide(NewNoop)) }
```

### 2.7 可观测性（调试日志与自定义日志）

在运行 `digen` 时启用 `-debug` 标志，生成的代码将在每个提供者和调用者周围插入 `Logf` 调用。

在 `main.go` 中运行时覆盖 `Logf`：

```go
var Logf = log.Printf // 在 dig_gen.go 中声明

func main() {
    Logf = myLogger.Printf // 自定义 logger
    // ...
}
```

### 2.8 未使用提供者策略

- **`error`**（默认） – 如果存在未使用的提供者，生成失败。
- **`ignore`** – 保留未使用的提供者，使用 `_ = fn()`。
- **`drop`** – 完全移除未使用的提供者。

```bash
digen -unused=drop -out di_gen.go
```

### 2.9 包别名策略

- **`full`**（默认） – `addr_handler`、`user_handler`
- **`short`** – `handler`、`handler2`
- **`obfuscated`** – `a`、`b`、`c1`

### 2.10 使用 `dig.Module` 的模块化代码组织

对于较大的项目，每个模块定义自己的 `Module()` 函数，返回 `dig.Option`。模块可以**嵌套** —— 一个模块可以包含其他模块，从而实现层级化组织。

**项目结构**：

```
myapp/
|-- di.go                         # 顶层组合
|-- main.go
|-- internal/
|   |-- db/
|   |   `-- module.go             # db.Module()
|   |-- server/
|   |   `-- module.go             # server.Module() – 可包含 db.Module()
|   |-- logger/
|   |   `-- module.go             # logger.Module()
|   `-- monitoring/
|       `-- module.go             # monitoring.Module() – 可包含 logger.Module()
`-- pkg/
    `-- common/
        `-- timeout.go
```

**示例：嵌套模块**

**`internal/db/module.go`**：
```go
package db

import "github.com/shanjunmei/dig"

func Module() dig.Option {
    return dig.Module(
        dig.Provide(NewConfig),
        dig.Provide(NewConnection),
        dig.Invoke(func(db *DB) error { return db.Ping() }),
    )
}
```

**`internal/logger/module.go`**：
```go
package logger

import "github.com/shanjunmei/dig"

func Module() dig.Option {
    return dig.Module(
        dig.Provide(New),
        dig.Invoke(Init),
    )
}
```

**`internal/monitoring/module.go`**（嵌套模块）：
```go
package monitoring

import (
    "myapp/internal/logger"
    "github.com/shanjunmei/dig"
)

func Module() dig.Option {
    return dig.Module(
        logger.Module(),
        dig.Provide(NewMetricsCollector),
        dig.Provide(NewHealthChecker),
        dig.Invoke(StartMetricsServer),
    )
}
```

**`internal/server/module.go`**（嵌套模块）：
```go
package server

import (
    "myapp/internal/db"
    "myapp/internal/monitoring"
    "github.com/shanjunmei/dig"
)

func Module() dig.Option {
    return dig.Module(
        db.Module(),
        monitoring.Module(),
        dig.Provide(New),
        dig.Provide(NewRouter),
        dig.Invoke(RegisterRoutes),
    )
}
```

**`di.go`** – 顶层组合：
```go
//go:build digen
package main

import (
    "myapp/internal/server"
    "myapp/internal/logger"
    "myapp/pkg/common"
    "github.com/shanjunmei/dig"
)

func InitApp() func(context.Context) error {
    return dig.Build(
        server.Module(),
        dig.Supply(common.DefaultTimeout),
        dig.Provide(NewGlobalService),
        logger.Module(), // 如果尚未被传递包含，则安全
        dig.Invoke(StartGlobalWorker),
    )
}
```

**嵌套的好处**：
- 将复杂的依赖层次封装在模块内部。
- 模块可以独立重用和组合。
- 即使项目增长，顶层 `di.go` 也保持整洁。

**重要**：不要重复包含同一个模块（无论是直接还是间接）—— 生成器会报告重复提供错误。设计模块层次时，确保每个模块只被包含一次。

---

## 3. 完整示例

[`example/`](./example) 目录包含一个综合演示，涵盖所有特性：

- 跨包依赖
- 泛型类型和函数
- 不同路径下的同名模块（`user/repository` 和 `role/repository`）
- 模块嵌套
- 外部参数（带参数的 `InitApp`）
- 带类型转换的 Supply
- 闭包使用
- 调试日志
- 构建标签
- 包别名策略

运行示例：

```bash
cd example
go generate ./...
go run .
```

请参考示例代码，了解一个功能完整的、贴近真实项目的依赖注入设置。

---

## CLI 参数（完整参考）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-out` | `di_gen.go` | 输出文件名 |
| `-unused` | `error` | 未使用提供者的处理方式：`error`、`ignore`、`drop` |
| `-debug` | `false` | 在生成的代码中启用调试日志（使用 `Logf`） |
| `-alias` | `full` | 包别名策略：`short`、`full` 或 `obfuscated` |

> **注意：** 当使用 `./...`（多包模式）调用 `digen` 时，`-out` 参数会被忽略，每个包目录下使用 `dig_gen.go`。

---

## 从 v1.0.4 升级

在 v1.0.4 及更早版本中，`InitApp()` 返回 `*dig.App`，并具有 `Run` 方法：

```go
app := InitApp()
if err := app.Run(context.Background()); err != nil {
    log.Fatal(err)
}
```

在 **v1.0.5** 中，`InitApp()` 直接返回 `func(context.Context) error`：

```go
run := InitApp()
if err := run(context.Background()); err != nil {
    log.Fatal(err)
}
```

**迁移步骤：**

1. 将 `app.Run(ctx)` 改为 `run(ctx)`，其中 `run := InitApp()`
2. 删除代码中所有对 `dig.App` 类型的引用
3. 更新 `di.go` 中的函数签名，从 `func InitApp() *dig.App` 改为 `func InitApp() func(context.Context) error`
4. 运行 `go generate` 重新生成 `di_gen.go`
5. 运行 `go mod tidy` 更新依赖

**为何做出此更改？** – 从 v1.0.5 开始，生成的代码在运行时不再导入 `dig` 包。这彻底消除了运行时依赖，从而产生更小的二进制文件并实现零反射开销。

---

## 与其他 DI 工具对比

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| **方式** | 代码生成（编译时） | 代码生成（编译时） | 运行时反射 |
| **代码生成工作流** | ✅ `digen` CLI | ✅ `wire` CLI | N/A |
| **零运行时反射** | ✅ | ✅ | N/A |
| **零运行时依赖** | ✅ | ✅ | N/A（依赖 `fx` 运行时） |
| **依赖验证时机** | 生成时 | 生成时 | 运行时 |
| **专用 `Supply` API** | ✅（`dig.Supply`） | ✅（`wire.Value`/`wire.InterfaceValue`） | ✅（`fx.Supply`） |
| **直接值注入成本** | 零（编译时） | 零（编译时） | 运行时反射开销 |
| **闭包安全检查** | ✅（捕获检查） | ⚠️（无检查） | N/A |
| **包装类型支持** | ✅ | ⚠️（手动） | N/A |
| **内置 `Invoke`** | ✅ | N/A | ✅（生命周期钩子） |
| **模块组合** | ✅（`dig.Module`，支持嵌套） | ✅（`wire.NewSet`，支持嵌套） | ✅（`fx.Module`） |
| **泛型支持** | ✅（编译时，完整） | ✅（需显式实例化） | ✅（运行时，通过反射） |
| **未使用提供者策略** | 3 种模式 | 仅 `drop` | N/A |
| **内置调试日志** | ✅（可运行时覆盖） | ⚠️（手动） | ✅（链路追踪） |
| **外部依赖** | 无（仅标准库） | 无 | 较多 |
| **生成代码尺寸** | 紧凑 | 冗长 | N/A |
| **生成性能** | 快速（AST 重写） | 较慢（完整类型检查） | N/A |
| **学习曲线** | 低 | 中等 | 高 |

---

## 许可证

MIT – 详见 [LICENSE](LICENSE) 文件。
