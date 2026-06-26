# dig — 编译时依赖注入（Go）

English | [中文文档](./README_zh.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/shanjunmei/dig.svg)](https://pkg.go.dev/github.com/shanjunmei/dig)
[![Go Report Card](https://goreportcard.com/badge/github.com/shanjunmei/dig)](https://goreportcard.com/report/github.com/shanjunmei/dig)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **📢 版本说明**
> - **v1.0.4** – 最后一个保留 `*dig.App` 结构体的版本
> - **v1.0.5** – `InitApp()` 返回 `func(context.Context) error`，零运行时依赖
> 迁移指南见 [从 v1.0.4 升级](#从-v104-升级)。

**dig** 是一个基于代码生成的 Go 依赖注入容器。  
它在**编译时**完成所有依赖解析，生成纯 Go 源代码 – **无反射**、无运行时魔法，纯粹的原生 Go 风格。

---

## 为什么选择 dig？

- **零反射** – 启动性能等同于手写初始化代码。
- **静态安全** – 缺失或循环依赖在 `go generate` 期间就会被捕获，不会留到运行时。
- **极简 API** – 仅 4 个核心函数：`Build`、`Provide`、`Supply`、`Invoke`。
- **内置 `Supply`** – 直接注入包级全局变量，无需繁琐封装。
- **包装类型** – 定义轻量别名来解决基础类型注入冲突。
- **内置 `Invoke`** – 所有启动/注册逻辑在所有 Provider 就绪后执行。
- **可观测性** – 可选的调试日志，带 `before/after` 标记；`Logf` 可在运行时覆盖。
- **未使用 Provider 策略** – 可选择 `error`（默认）、`ignore` 或 `drop`。
- **零运行时依赖** – 生成的代码在运行时不再导入 `dig` 包。
- **二进制体积小** – 无内嵌反射或框架运行时。
- **学习曲线低** – 仅 4 个 API 和几条清晰的约束。

---

## 安装

```bash
go get github.com/shanjunmei/dig@v1.0.5
go install github.com/shanjunmei/dig/cmd/digen@latest
```

需要 Go 1.21+。

---

## 1. 基础用法

使用 **dig** 最简单的方式涉及两个文件。

### 1.1 容器定义（`di.go`）

这个文件是您需要编写的**唯一** DI 编排文件。它使用 `//go:build digen` 构建标签，因此仅在代码生成期间被解析。

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
        // 1) 普通函数构造函数
        dig.Provide(NewConfig),
        dig.Provide(NewDB),

        // 2) 直接提供一个值（对常量或全局变量很有用）
        dig.Supply(DefaultTimeout),

        // 3) 使用内联闭包提供 – 只要它只使用常量字面量或已供应的全局变量即可。
        dig.Provide(func(timeout Timeout) *Server {
            return NewServer(timeout)
        }),

        // 4) Invoke 在所有 Provider 就绪后运行。
        //    它可以返回一个 error，该错误会传递给调用方。
        dig.Invoke(func(srv *Server) error {
            return srv.Run()
        }),
    )
}
```

### 1.2 业务逻辑（`main.go`）

此文件包含所有类型、构造函数和 `main()` 函数。它**没有**构建标签。

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
# 只生成当前包（默认）
digen

# 递归生成所有子包
digen ./...
```

或通过 `go generate`（如果你的 `di.go` 中包含 `//go:generate` 指令）：

```bash
go generate ./...
```

生成后，构建并运行你的应用：

```bash
go run .
```

生成器会在每个包含 `dig.Build` 调用的包目录下输出 `dig_gen.go`。每个生成的文件都是自包含的，运行时不再依赖 `dig` 包。

---

## 2. 高级用法

### 2.1 Supply – 注入已有值

您已经在基础示例中看到了 `Supply`——它注入一个包级变量。您可以将它用于任何全局变量、常量或运行时计算的值。

```go
dig.Supply(globalDBConn)
dig.Supply(apiKey)
```

### 2.2 使用包装类型解决基础类型冲突

如果您需要多个 `bool`、`string` 或 `time.Duration` 值，将它们包装成不同的类型以避免重复 provider 错误。

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

生成器将每个包装类型视为唯一，因此不会发生冲突。

### 2.3 闭包约束（关键）

当您在 `dig.Provide` 中编写内联匿名函数时，**绝不能**捕获 `InitApp()` 外层作用域中的变量。

✅ **正确** – 仅使用常量字面量：
```go
dig.Provide(func() QueryTimeout { return QueryTimeout(5 * time.Second) })
```

❌ **错误** – 捕获了局部变量 `t`：
```go
t := 5 * time.Second
dig.Provide(func() QueryTimeout { return QueryTimeout(t) })
// 生成器错误：cannot capture local variable "t"
```

**为什么？**  
生成器会提取闭包体并将其提升为 `di_gen.go` 中的一个**包级别**函数。这个新函数**无法**访问 `InitApp()` 的栈帧。如果闭包捕获了 `InitApp()` 中的局部变量，该变量在包作用域中不存在，会导致“未定义符号”的编译错误。

**重要**：即使捕获的值是常量，它仍然绑定在 `InitApp` 的作用域内。只有**常量字面量**（如 `true`、`3`、`"hello"`）和**包级变量/常量**是允许的，因为它们在包级别是可解析的。

**如果您需要一个运行时计算的值，请将其定义为包级变量并使用 `dig.Supply` 注入。**

### 2.4 条件逻辑的有效范围（哪里有效，哪里无效）

因为 dig 是**代码生成器**而非运行时框架，它对你写的 `di.go` 和模块文件进行**静态分析**。这意味着生成器会把你的代码当作文本读取，但**不会执行它**。因此，条件逻辑的有效性取决于你将它放在哪里。

#### ✅ `Provide`/`Invoke` 闭包内的条件（有效）

传递给 `dig.Provide` 或 `dig.Invoke` 的闭包体会被**完整复制**到生成的代码中。闭包内的所有条件逻辑都会在**运行时**正常执行。

**示例（经典的条件注入）：**
```go
dig.Provide(func(t QueryTimeout, useMySQL UseMySQL, cache EnableCache) Store {
    if useMySQL {
        return NewMySQLStore(t)
    }
    return NewRedisStore(cache)
})
```

生成器会将整个闭包体复制到生成的函数（如 `__p_xxx`）中。运行时，`useMySQL` 决定创建哪个 Store。

**这是处理条件逻辑的推荐方式。**

#### ❌ `Module()` 函数体内的条件（无效）

`Module()` 函数会被静态解析。生成器**不会执行**其中的任何 `if` 语句、循环或分支。无论这些调用出现在哪个分支，所有 `dig.Provide`、`dig.Invoke`、`dig.Supply` 调用都会被提取并纳入依赖图。

**不符合预期的示例：**
```go
func Module() dig.Option {
    if enableCache {
        return dig.Module(
            dig.Provide(NewCache),
            dig.Invoke(StartCache),
        )
    }
    return dig.Module(
        dig.Provide(NewNoop),
        dig.Invoke(StartNoop),
    )
}
```

生成器会解析**两个分支**，并将所有 Provider（`NewCache`、`NewNoop`、`StartCache`、`StartNoop`）都注册到依赖图中，而不管 `enableCache` 的值。该条件在生成期间从未被求值。

**如果确实需要条件性包含模块**，请使用**构建标签（build tags）**：

```go
// module_cache.go
//go:build enable_cache
package mod
func Module() dig.Option { return dig.Module(dig.Provide(NewCache), dig.Invoke(StartCache)) }

// module_noop.go
//go:build !enable_cache
package mod
func Module() dig.Option { return dig.Module(dig.Provide(NewNoop), dig.Invoke(StartNoop)) }
```

然后在 `di.go` 中：
```go
func InitApp() func(context.Context) error {
    return dig.Build(
        mod.Module(), // 构建标签决定编译哪个文件
    )
}
```

#### ✅ `Invoke` 闭包内的条件（有效）

与 `Provide` 闭包相同——闭包体会被完整复制，在运行时执行。

```go
dig.Invoke(func(config Config) {
    if config.Debug {
        log.Println("debug mode enabled")
    }
})
```

#### ❌ `dig.Module` 中的 IIFE（不推荐）

你可能尝试在 `dig.Module` 中使用立即调用函数表达式（IIFE）：

```go
dig.Module(
    func() dig.Option {
        if someCondition {
            return dig.Provide(NewCache)
        }
        return dig.Provide(NewNoop)
    }(),
)
```

这**不工作**，因为 `someCondition` 在生成期无法被求值。生成器会解析两个分支并注册两个 Provider。请避免这种用法。

#### 总结表

| 位置 | 条件逻辑是否有效 | 原因 |
|------|------------------|------|
| `Provide`/`Invoke` 闭包内部 | ✅ 有效 | 闭包体被复制，在运行时执行 |
| `Module()` 函数体内部 | ❌ 无效 | 生成器不执行控制流 |
| `dig.Module` 参数中的 IIFE | ❌ 无效 | 条件在生成期无法求值 |
| 使用构建标签 | ✅ 有效 | 由 Go 在编译时选择 |

**经验法则：**
- 将运行时决策放在 `Provide`/`Invoke` 闭包**内部**。
- 使用**构建标签**进行编译时的模块选择。
- 保持 `Module()` 函数**纯粹**——只包含 `dig.Module` 调用和 `return`。

### 2.5 可观测性（调试日志与自定义日志）

**启用调试日志** 使用 `-debug` 标志。

直接运行生成器：
```bash
digen -debug -out di_gen.go
```

或者在 `di.go` 的 `//go:generate` 指令中添加 `-debug`：
```go
//go:generate go run -mod=mod github.com/shanjunmei/dig/cmd/digen -debug -out di_gen.go
```

启用后，生成的代码会插入 `Logf` 调用，类似这样：

```go
Logf("[PROVIDE] before: %s\n", "main.NewConfig")
v0, err := NewConfig()
if err != nil {
    Logf("[PROVIDE] failed: %s: %v\n", "main.NewConfig", err)
    panic(err)
}
Logf("[PROVIDE] after: %s\n", "main.NewConfig")
```

运行时输出类似：

```
[PROVIDE] before: main.NewConfig
[PROVIDE] after:  main.NewConfig
[PROVIDE] before: main.NewDB
[PROVIDE] after:  main.NewDB
[INVOKE]  before: main.(*Server).Run
[INVOKE]  after:  main.(*Server).Run
```

**运行时覆盖 `Logf`** – 生成的文件声明：

```go
var Logf = log.Printf
```

您可以在 `main.go` 中调用 `InitApp()` 之前覆盖它：

```go
package main

import (
    "log"
    "os"
)

func main() {
    customLogger := log.New(os.Stdout, "[MYAPP] ", log.LstdFlags|log.Lshortfile)
    Logf = customLogger.Printf

    run := InitApp()
    if err := run(context.Background()); err != nil {
        customLogger.Fatalf("app failed: %v", err)
    }
}
```

**无外部依赖** – `Logf` 默认使用标准 `log`。

### 2.6 未使用 Provider 策略

- **`error`**（默认）– 如果存在未使用的 provider，生成失败。
- **`ignore`** – 保留未使用的 provider，使用 `_ = fn()`。
- **`drop`** – 完全从生成代码中移除未使用的 provider。

```bash
digen -unused=drop -out di_gen.go
```

### 2.7 包别名策略

- **`full`**（默认）– `addr_handler`、`user_handler`
- **`short`** – `handler`、`handler2`
- **`obfuscated`** – `a`、`b`、`c1`

### 2.8 使用 `dig.Module` 进行模块化代码组织

对于大型项目，每个模块定义自己的 `Module()` 函数，返回 `dig.Option`。模块可以**嵌套**——一个模块可以包含其他模块，实现分层组织。

**项目结构**：

```
myapp/
|-- di.go                         # 顶层组合
|-- main.go
|-- internal/
|   |-- db/
|   |   `-- module.go             # db.Module()
|   |-- server/
|   |   `-- module.go             # server.Module() – 可能包含 db.Module()
|   |-- logger/
|   |   `-- module.go             # logger.Module()
|   `-- monitoring/
|       `-- module.go             # monitoring.Module() – 可能包含 logger.Module()
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

**`di.go`** – 顶层组合（混合模块与普通 provider）：
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
        logger.Module(), // 如果尚未被传递包含，则是安全的
        dig.Invoke(StartGlobalWorker),
    )
}
```

**嵌套的好处**：
- 将复杂的依赖层次封装在模块内部。
- 模块可以独立复用和组合。
- 即使项目增长，顶层的 `di.go` 仍然保持整洁。

**重要**：不要重复包含同一个模块（直接或间接）——生成器会报告重复 provider 错误。设计模块层次时确保每个模块只被包含一次。

---

## CLI 参数（完整参考）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-out` | `di_gen.go` | 输出文件名 |
| `-unused` | `error` | 未使用 Provider 的处理方式：`error`、`ignore`、`drop` |
| `-debug` | `false` | 在生成代码中启用调试日志（使用 `Logf`） |
| `-alias` | `full` | 包别名策略：`short`、`full` 或 `obfuscated` |

> **注意**：当 `digen` 使用 `./...`（多包模式）时，`-out` 参数会被忽略，每个包目录下统一使用 `dig_gen.go`。

---

## 从 v1.0.4 升级

在 v1.0.4 及更早版本中，`InitApp()` 返回 `*dig.App`，包含 `Run` 方法：

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
2. 移除代码中所有对 `dig.App` 类型的引用
3. 更新 `di.go` 中的函数签名：`func InitApp() *dig.App` → `func InitApp() func(context.Context) error`
4. 运行 `go generate` 重新生成 `di_gen.go`
5. 运行 `go mod tidy` 更新依赖

**为什么做这个改动？** – 从 v1.0.5 开始，生成的代码在运行时不再导入 `dig` 包。这彻底消除了运行时依赖，带来更小的二进制体积和零反射开销。

---

## 与其他 DI 工具的对比

| 特性 | dig | Google Wire | Uber dig / FX |
|------|-----|-------------|---------------|
| **方式** | 代码生成（编译时） | 代码生成（编译时） | 运行时反射（无生成） |
| **代码生成工作流** | ✅ `digen` CLI | ✅ `wire` CLI | ❌ 不适用 |
| **零运行时反射** | ✅ | ✅ | ❌ |
| **零运行时依赖** | ✅ | ✅ | ❌ |
| **依赖校验** | 生成时 | 生成时 | 运行时 |
| **专用 `Supply` API** | ✅ | ❌ | ❌ |
| **闭包安全强制** | ✅（捕获检查） | ⚠️（无检查） | N/A |
| **包装类型支持** | ✅ | ⚠️（手动） | ❌ |
| **内置 `Invoke`** | ✅ | ❌ | ✅（生命周期钩子） |
| **`dig.Module` 组合** | ✅（支持嵌套） | ❌ | ✅（fx.Module） |
| **未使用 Provider 策略** | 3 种模式 | 仅 `drop` | N/A |
| **内置调试日志** | ✅（可运行时覆盖） | ⚠️（手动） | ✅（追踪） |
| **外部依赖** | 无（仅标准库） | 无 | 大量 |
| **生成代码体积** | 紧凑 | 冗长 | N/A |
| **生成性能** | 快（AST 重写） | 较慢（完整类型检查） | N/A |
| **学习曲线** | 低 | 中 | 高 |

---

## 许可证

MIT – 详见 [LICENSE](LICENSE) 文件。
