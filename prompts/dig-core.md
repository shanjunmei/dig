# dig 核心开发技能（Core）

> `system_prompt_dig.md` 的「核心用法」子模块，面向 `github.com/shanjunmei/dig` 编译期 DI 库。
> 覆盖 API、语法约束、digen CLI 与标准模板。历史版本差异见 `dig-migration.md`；排错见 `dig-troubleshooting.md`；横向对比见 `dig-comparison.md`。

## 1. 基础信息

- 定位：基于代码生成的编译期 IoC 容器，零运行时反射，生成代码零 dig 运行时依赖
- Go 版本：1.25+
- 默认生成文件名：`dig_gen.go`（非 `di_gen.go`）
- 协议：MIT

```bash
go get github.com/shanjunmei/dig@latest
go install github.com/shanjunmei/dig/cmd/digen@latest
```

## 2. 五大核心 API

1. `dig.Build(opts ...Option)`：组装 DI 容器，返回启动函数
2. `dig.Provide(constructors ...any)`：注册构造函数
3. `dig.Supply(values ...any)`：注入任意运行时/常量值（Wire.Value 仅接受常量/复合字面量，不能调用函数；dig.Supply 无此限制）
4. `dig.Invoke(functions ...any)`：依赖就绪后执行启动逻辑，支持返回 error
5. `dig.Module(opts ...Option)`：模块分组，支持多层嵌套复用，自动检测重复模块

## 3. 命名多实例注入

- 定义：`dig.Provide(func() (mainDB, reportDB *sql.DB, err error) {...})` 或 `dig.Supply(mainDB)`（变量名即实例名）
- 消费：`dig.Invoke(func(mainDB *sql.DB) {...})`（参数名匹配实例名）
- 歧义：多实例存在但消费者未指定参数名 → 报歧义错误并列出可用名称

## 4. 强制语法约束

1. **闭包捕获限制**：Provide/Invoke 匿名闭包禁止捕获 InitApp 局部变量，仅允许包级变量、字面量
2. **DI 配置隔离**：源 `di.go` 应带 `//go:build digen`（约定）：digen 仅识别 `di.go` 入口、不读取所有源文件；`go build` 跳过带该 tag 的源文件，避免正常构建出现重复 `InitApp` 声明。生成的 `dig_gen.go` 由 digen 写入 `//go:build !digen`（硬编码，安全）。禁止在此定义业务结构体/构造函数/自定义类型/全局常量。源文件携带 `//go:build digen` 由 digen 在生成期校验
3. **基础类型冲突**：用包装类型区分（如 `type UseMySQL bool`）
4. **泛型实例化**：必须显式实例化，如 `dig.Provide(NewStore[int])`
5. **条件分支**：闭包内允许运行时 if；Module() 外层禁止 if（所有分支同时注册），编译期切换用 build tag
6. **InitApp 入参**：自动转为 Supply 注入，无需手动捕获

## 5. digen CLI

| 参数 | 默认值 | 作用 |
|------|--------|------|
| `-out` | dig_gen.go | 生成文件名，`digen ./...` 下失效 |
| `-unused` | error | 未使用构造器策略：error / ignore / drop |
| `-debug` | false | 调试日志（含别名映射诊断） |
| `-alias` | full | 别名策略：full / short / obfuscated / numeric |
| `-inline` | false | 简单闭包内联为 IIFE（身份闭包始终塌缩为类型转换，与本 flag 无关） |
| `-version` | false | 打印版本信息 |
| `-typecheck` | true | 生成后类型检查产出代码以捕获内部生成器 bug（大型 `./...` 可关） |
| `-cache` | false | 缓存提取出的 IR 到磁盘，未改动包跳过提取/类型检查 |
| `-cachedir` | "" | IR 缓存目录（默认 `os.TempDir()/digen-ir-cache`；仅 `-cache` 生效） |

子命令：`init`（脚手架 di.go）、`check`（仅校验不写文件）、`graph`（Mermaid 依赖图）、`explain <type>`（解析路径）、`completion <shell>`（bash/zsh/fish 补全脚本）。

## 6. 标准模板

```go
//go:build digen
package main

import (
    "context"
    "github.com/shanjunmei/dig"
)

func InitApp() func(context.Context) error {
    return dig.Build(
        dig.Provide(NewConfig),
        dig.Provide(NewDB),
        dig.Supply(DefaultTimeout),
        dig.Provide(func(t Timeout) *Server { return NewServer(t) }),
        dig.Invoke(func(srv *Server) error { return srv.Run() }),
    )
}
```

```bash
digen ./...
go run .
```

```go
// 自定义日志覆盖（dig_gen.go 自动生成全局 Logf）
func main() {
    Logf = log.Printf // 替换为 zap/logrus
    run := InitApp()
    if err := run(context.Background()); err != nil { panic(err) }
}
```

## 7. 禁止行为

1. 不混淆 `go.uber.org/dig`（运行时 DI）与本库 `shanjunmei/dig`（编译期 DI）
2. 不使用 Wire/Fx 专属 API 写 dig 代码
3. 不给出违反闭包捕获规则的示例
4. 不编造不存在的 API、CLI 参数
5. 不否认 dig 支持多实例注入（命名参数已支持）
