# 增强：版本信息系统与 Mage 构建支持

## 问题描述

digen 之前没有版本信息系统，用户无法通过 `-version` 参数查看当前版本。构建过程也没有自动化工具，需要手动执行 `go build`。此外，通过 `go install` 安装的二进制文件无法自动获取版本信息。

## 改进方案

### 1. 新增版本信息模块（`cmd/digen/version.go`）

实现了灵活的版本信息系统，支持多种信息来源和优先级策略：

**版本信息来源优先级**：
1. **ldflags 注入**（优先）：通过 `-ldflags "-X main.Version=v1.0.0 ..."` 由构建工具注入
2. **VCS 信息回退**：当 ldflags 为空时，自动从 `debug.ReadBuildInfo()` 读取 Go 嵌入的 VCS 信息

**版本字符串格式**：
```
digen v1.0.11+13 (dirty)
  commit: 0f16d86
  built:  2026-07-29 18:48:05 -07:00
```

**支持 git describe 格式解析**：
| git describe 输出 | 解析结果 |
|---|---|
| `v1.0.11` | tag=v1.0.11 |
| `v1.0.11-dirty` | tag=v1.0.11, dirty=true |
| `v1.0.11-13-g0f16d86` | tag=v1.0.11, commits=13, commit=0f16d86 |
| `v1.0.11-13-g0f16d86-dirty` | tag=v1.0.11, commits=13, dirty=true |

**关键特性**：
- 无默认值：不强制设默认版本号，缺失时留空
- Go 原生支持：通过 `debug.ReadBuildInfo()` 自动获取 `go install` 安装版本的 VCS 信息
- 缓存机制：`getVersion()` 使用包级缓存，避免重复解析
- 多种日期格式：`formatDate` 支持 RFC3339 等多种日期格式解析

### 2. 新增 `-version` 命令行参数（`cmd/digen/main.go`）

```go
showVersion := flag.Bool("version", false, "print version information and exit")
// ...
if *showVersion {
    fmt.Println(versionString())
    return
}
```

### 3. 新增 Mage 构建系统（`magefile.go`）

提供了完整的自动化构建任务：

| 任务 | 说明 |
|---|---|
| `mage build` | 构建二进制，自动从 git 获取版本号并注入 ldflags |
| `mage install` | 安装到 `$GOPATH/bin`，注入版本号 |
| `mage generate` | 运行 digen 生成示例代码 |
| `mage test` | 运行测试（`go test -v -race ./...`） |
| `mage vet` | 静态检查（`go vet ./...`） |
| `mage clean` | 清理 `bin/` 目录和 `*_gen.go` 文件 |
| `mage default` | 默认执行 `build` |

**构建时自动注入**：
```go
func ldflags() string {
    version, commit, buildDate := getVersionInfo()
    return fmt.Sprintf("-X 'main.Version=%s' -X 'main.Commit=%s' -X 'main.BuildDate=%s'",
        version, commit, buildDate)
}
```

**从 git 获取版本信息**：
- `git describe --tags --dirty`：获取带 dirty 状态的版本标签
- `git rev-parse HEAD`：获取完整 commit hash

### 4. 新增 `.gitignore`

忽略构建产物、IDE 配置、OS 临时文件等：
- `bin/`、`*.exe`、`*.test` 等构建产物
- `.idea/`、`.vscode/` 等 IDE 配置
- `go.work`、`*.log` 等临时文件

## 涉及文件

- `cmd/digen/version.go`（新增）：版本信息解析和格式化
- `cmd/digen/main.go`（修改）：添加 `-version` 参数
- `magefile.go`（新增）：Mage 构建系统
- `.gitignore`（新增）：忽略规则

## 向后兼容性

- 现有命令行参数保持不变
- 不使用 mage 时，`go build ./cmd/digen` 仍然可以正常工作（只是没有版本号）
- 使用 `go install github.com/shanjunmei/dig/cmd/digen@latest` 安装时，Go 1.18+ 会自动嵌入 VCS 信息，版本号可正常显示
