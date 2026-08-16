# shanjunmei/dig 项目 DI 全面排查报告

> 审计对象：`github.com/shanjunmei/dig`（编译期 DI 框架本体，Go 1.25.0，框架版本自 v1.0.15）
> 审计范围：使用 dig 的**消费代码**（`example/**`、`cmd/digen/di.go`）
> 审计方法：6 阶段流程（侦察 → 5 并行狩猎 Agent → 对抗性质疑 → 可达性追踪 → 去重分级 → 可选修复验证）
> 结论日期：2026-08-16

---

## 0. ⚠️ 关键前提说明（与任务书假设不一致，务必先读）

任务书的部分前提与本项目实际不符，已核实并校正：

| 任务书假设 | 项目实际情况 | 影响 |
|---|---|---|
| 项目是 dig 的**下游消费者**，需从 v1.0.5 迁移 | 本项目**就是 dig 框架本身**（自导入 v1.0.15），`example/` 是其官方示例 | v1.0.5 迁移风险 **N/A**；所有示例已用 `func(context.Context) error` |
| 排查 `//go:generate digen` 注释的 `_dig.go` 文件 | 仓库**无 `_dig.go` 文件**，也**无 `//go:generate digen` 注释** | 改用真实约定审计（见下） |
| digen 通过 `//go:generate digen` 触发 | 真实约定：`di.go` 源文件带 `//go:build digen`；`//go:generate go run .../cmd/digen` 指令写在**生成的 `dig_gen.go`** 里 | 13 个含 `dig.Build` 的 `di.go` 全部正确携带 `//go:build digen` |

**排除项**：`example/gen_failures/**`（20+ 个包）是**故意的失败夹具**，由 `example/gen_failures/gentest/gen_failures_test.go` 覆盖，用于验证 digen 在生成期报错。它们被设计为失败，**不计入 bug**。

---

## 1. 阶段一：项目 DI 图谱摘要

### 1.1 版本与构建约束
- `go.mod`：`go 1.25.0`（≥ 1.21 ✅）；直接依赖仅 `golang.org/x/tools`。
- dig 版本：模块自身 v1.0.15（README/CHANGELOG），无 `*dig.App` 残留，无 `go.uber.org/dig` 混入。
- Build 标签：所有 `di.go`（`dig.Build` 所在）均带 `//go:build digen`；生成的 `dig_gen.go` 带 `//go:build !digen`。digen 在生成期强制校验此约束（`internal/extractor/buildtag.go:39`）。

### 1.2 核心 Provider / Invoke / Module 图谱（工作示例）

**`cmd/digen/di.go` — `InitApp(cfg *config.Config)`（生成器自身容器，真实生产代码）**
```
Provide: logger.NewLogger · loader.NewPackageLoader
         (cfg *config.Config) → string           // 闭包，参数非捕获
         (aliasType string) → alias.AliasStrategy // 含 log.Fatalln（见 P2）
Provide: app.NewApp · generator.NewGenerator · processor.NewProcessor
Invoke:  func(a *app.App) error { return a.Run() }
```

**`example/app/di.go` — `InitApp(cfg *common.Config, log *logger.Logger)`**
```
Build: setup.Full()                       // 嵌套 user/role/cache/db 四模块
Supply: "app-v2"  (无名 string，未消费 → P3)
Invoke: func(s *user.Store[string], cfg, log) error
Invoke: func(stringCache *cache.Cache[string])
```
**`example/setup/full.go` — `Full() / Basic()` 模块（无 build tag，被 app 内联）**
```
Module: user.Module · role.Module · cache.Module · db.Module
闭包 Provide: *user.Store[string] · db.Pinger(接口) · any · *common.Config · db.DB
Invoke: 跨包命名实例消费 (primaryDB/replicaDB/userRedis/Index) · 运行时 if/else 分支(允许)
```

**子模块图谱**
| 模块 | Provider | 命名实例 / Supply | Invoke |
|---|---|---|---|
| `user.Module` | `NewStore[int]` · `repository.Module` · `func()(str string,err)` | `str`(string) | `ProcessStore[int]` |
| `role.Module` | `NewServer` · `repository.Module` | `Supply(100)`(int) · `Supply(Config("production"))` | `Config` · `Server.Run()` |
| `db.Module` | `func()(primaryDB/replicaDB *DB,err)` · `func()(userRedis/sessionRedis *RedisClient)` | `Supply(Index)`(RedisDbIndex) | 命名实例消费 |
| `cache.Module` | `NewCache[string]` · `NewCache[int]` · `func()*Cache[bool]` | — | 消费 string/int 缓存（bool 未消费 → P3） |
| `user/repository.Module` | `NewRepository[string]` | — | `r.Print()` |
| `role/repository.Module` | `NewRepository[int]` | — | `r.Add/Print` |

其余示例（`app_basic`/`app_debug`/`app_edge`/`app_xpkg_generic`/`app_gen_test`/`app_runtime_err`/`closure_param`/`context_alias`/`shadow_err`/`shadow_freevar`/`supply_param`）均为结构类似的独立 `InitX`，泛型实例化完整、无环、无缺供。

---

## 2. 阶段二~四：5 专项 Agent 结论 + 对抗性质疑 + 可达性

| Agent | 维度 | 结论 | 质疑结果 | 可达性 |
|---|---|---|---|---|
| A | 版本兼容 | 全清：无 `*dig.App`、签名均 `func(ctx) error`、Go≥1.21、无 Uber-dig 混入 | — | — |
| B | 语法合规（5 条 digen 硬约束） | 全清：无闭包捕获、无 DI 配置隔离违规、无基础类型碰撞、泛型全实例化、无非法条件分支 | — | — |
| C | 依赖图谱 | 3 处未使用 Provider（P3）；无环/无缺供/无嵌套冲突 | 见阶段五 | 编译通过，未运行时解析 |
| D | 运行时风险 | 1 处：`cmd/digen/di.go:32` `log.Fatalln`（原判 P1） | 降级为 P2（见下） | `main.go:48` 调用，运行期可达 |
| E | 迁移兼容 | 全清：无 Fx/Wire 残留、无双 dig 包混淆 | — | — |

### 对抗性质疑（阶段三）
- **log.Fatalln 项**：仅当 `-alias` 传入非法值（非 `full/short/obfuscated/numeric`）时触发；flag 默认 `full`，正常路径永不命中。对 CLI 工具而言，非法配置即 `os.Exit` 属可接受 UX，且 digen 不校验 provider 函数体语义（非生成期硬错）。→ **[存疑]**，降级 **P2**（违反 dig「provider 应返回 error」的最佳实践，非功能缺陷）。
- **3 处未使用 Provider**：在默认 `-unused=ignore` 下仅为惰性死重，dig 运行时只解析 Invoke 链所需节点，不报错、不影响编译。→ **[误报]**（无功能性缺陷）；作为 P3 可维护性提示保留。

### 可达性追踪（阶段四）
| 问题 | go generate 解析 | go build / 运行期执行 | 是否真实 |
|---|---|---|---|
| log.Fatalln | 是（digen 自举） | 是（`main.go:48 → dig_provider_2` 必跑） | 真实但仅非法输入触发 |
| 未使用 Provider×3 | 是（successtest 重新生成） | 编译通过，运行期不解析 | 否（误报/死重） |

---

## 3. 阶段五：最终问题清单（去重 + 分级）

**P0（阻断性）：无。**
**P1（高危/运行时 panic/泄漏）：无。**
**P2（中危/最佳实践）：1 项。**

### P2-1 `cmd/digen/di.go:29-35` — provider 内 `log.Fatalln` 绕过 error 返回路径
```go
dig.Provide(func(_aliasType string) alias.AliasStrategy {
    aliasType, err := alias.ParseAliasType(_aliasType)
    if err != nil {
        log.Fatalln(err)          // ← 直接 os.Exit(1)，绕过 InitApp 的 error 返回
    }
    return alias.NewAliasStrategy(aliasType)
}),
```
**修复建议**：改为返回 error，让 dig 的错误传播机制生效：
```go
dig.Provide(func(_aliasType string) (alias.AliasStrategy, error) {
    aliasType, err := alias.ParseAliasType(_aliasType)
    if err != nil {
        return nil, err          // ← 交由 InitApp 的 error 返回链处理
    }
    return alias.NewAliasStrategy(aliasType), nil
}),
```
> 注：仅影响非法 `-alias` 输入；当前为 CLI 可接受行为，属健壮性/一致性改进，非必须。

**P3（低危/建议）：3 项（均为死重，不影响构建与运行）**
- `example/cache/module.go:16` — `dig.Provide(func() *cache.Cache[bool]{...})` 未被任何 Invoke 消费。
- `example/app/di.go:31` — `dig.Supply("app-v2")` 无名 string 未被消费（教学残留，可删除或补 `dig.Invoke(func(v string){_=v})`）。
- `example/app_debug/di.go:24` — 同上 `dig.Supply("app-debug")`。

---

## 4. 验证（阶段六前半，未做修复所以仅验证现状）

- `go build ./...` → **EXIT=0**，全模块（框架 + 消费代码 + 已提交 `dig_gen.go`）编译通过，确认**无 P0 构建失败**。
- 未执行 `go generate ./...`：会改写已提交的 `dig_gen.go`，属仓库变更，按只读审计原则跳过（如需执行验证，建议先 `git stash` 或在干净分支进行）。

---

## 5. 结论

该项目作为 dig 框架本体，其**消费代码（示例 + 生成器自身容器）整体质量很高**，5 条 digen 硬约束、版本兼容、迁移兼容均全面合规，**无 P0/P1 真实缺陷**。唯一可改进项为 P2 的 `log.Fatalln` 最佳实践问题（非必须）；3 处 P3 为无害死重。任务书关于「`_dig.go` / `//go:generate digen` / v1.0.5 迁移」的前提与本项目实际不符，已在校正后完成审计。

---

## 6. 后续补充（2026-08-16）— 框架错误诊断的硬化

本报告审计的是**消费代码**；其后框架本体的生成期错误诊断又做了硬化，与"用户违反 digen 契约却被误判为工具 bug"这一长期痛点直接相关：

- **`di.go` 只放接线的契约预检**：`internal/extractor/contract.go` 的 `checkContractVisibility` 在写文件前拦截"接线引用了定义在 `//go:build digen` 文件中的主包符号"，给出清晰的 `digen contract violation` 错误与 `💡 Fix:`，不再让这类契约违反事后表现为晦涩的 `undefined: X`。
- **生成后安全网分类**：`generator.go` 的 `typeCheckGenerated` 现将触发的 `undefined: X` 分类——X 属 digen 定义主包符号时为**契约违规**（与预检一致），否则为**内部生成器 bug**（预填 issue 链接）。修正了此前"把契约违反谎称为 internal generator bug"的误导措辞，并堵住 IR 缓存命中跳过预检的缺口。
- **构建约束检查收敛到 `internal/buildconstraint` 共享包**，消除 `extractor`/`generator` 间的重复实现。

这些硬化不影响本报告对消费代码"无 P0/P1 真实缺陷"的结论；唯 P2 的 `log.Fatalln` 项仍可按原建议择机改进。
