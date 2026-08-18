# dig 迁移与版本技能（Migration）

> `system_prompt_dig.md` 的「迁移/版本」子模块。集中存放历史版本差异，避免在主文档与核心技能中重复。

## 1. 从 v1.0.4 升级

- 旧：`app.Run(ctx)`
- 新：`run := InitApp(); run(ctx)`
- （v1.0.5 起 `InitApp()` 返回 `func(context.Context) error`，移除 `*dig.App`。）

## 2. 版本变更简表

| 版本 | 关键变更 |
|------|---------|
| v1.0.5 | 移除 `*dig.App`，`InitApp()` 返回 `func(context.Context) error` |
| v1.0.11 | 命名多实例注入；包别名解析修复（如 `go-redis/v9`） |
| v1.0.13 | `-version`/Mage 构建；Provide 闭包签名校验；结构化错误含 `💡 Fix:` |
| v1.0.14 | 闭包内联 `-inline`；跨包别名隔离；错误含 `file:line:col`；全局 Logger 统一诊断 |
| v1.0.15 | 类型包收集健壮性修复；移除 `-debug-aliases`（并入 `-debug`）；放开单 Module 限制 |
| v1.0.16 | ShadowGuard 变量遮蔽保护；未使用参数默认改为 `error` |
| v1.0.17 | 拦截闭包内未导出跨包调用；新增示例与 GitHub Pages 站点 |
| v1.0.18 | provider 禁 `context.Context` 参数；`//go:build digen` 生成期校验；`go/types` 安全网；IR 缓存 |

## 3. 当前关键特性（按引入版本，便于迁移判断）

- 命名多实例注入（v1.0.11+）
- 版本信息系统（v1.0.13+）：`-version`、Mage、结构化错误
- 闭包内联（v1.0.14+）：`-inline`
- 多 Module 支持（v1.0.15+）
- ShadowGuard（v1.0.16+）
- 闭包未导出跨包调用拦截（v1.0.17+）
- 生成期加固与诊断（v1.0.18+）

## 4. 从 Wire / Fx 迁移到 dig

1. 用对照表（见 `dig-comparison.md`）确认能力映射
2. 将运行时 `fx.Provide`/`fx.Invoke` 或 `wire.NewSet` 改写为 `dig.Build(...)` 内的 `dig.Provide`/`dig.Invoke`
3. 接口绑定：Wire `wire.Bind` / Fx `fx.As` → dig 身份闭包（内联为类型转换）
4. 多实例：Fx 命名/值组 → dig 命名参数
5. 删除运行时依赖：移除 `go.uber.org/fx`、`google/wire` 导入与 `app.Run` 等运行时启动代码
6. 在 di.go 加 `//go:build digen`，运行 `digen ./...` 生成 `dig_gen.go`
