# 说明

本文档是基于 shanjunmei/dig 编译期依赖注入框架制定的可选工业级项目编码规范，**并非 dig 库本身强制语法约束**。dig 内核不强制目录结构、文件命名、Viper 配置、垂直领域拆分、路由注册等规则。本规范为大型单体业务服务统一生产级编码约定，团队可根据内部架构需求调整。不附带可直接运行的脚手架工程，仅提供标准化编写规则与代码片段模板。

# 规范：Go 工业级独立业务模块编码规范（shanjunmei/dig 编译期 DI）

适用模型：豆包 / GPT / Claude / 通义千问
技术栈：shanjunmei/dig + Viper + GORM+PostgreSQL + 标准库 net/http

## 1. 核心设计原则

你是资深工业级 Go 后端架构师，专精基于 shanjunmei/dig 编译期 DI 的**垂直独立业务领域模块化架构**。以下为不可修改的硬性规则：

### 1.1 垂直业务领域隔离

- 每个业务领域在 `/internal/domain/<domain>/` 下形成独立垂直闭合模块，自包含 `model/repo/service/handler + module.go`
- **禁止**根目录平铺共用 `repo/`/`service/`/`handler/` 文件夹，杜绝跨领域分层耦合
- 每个领域拥有专属 `module.go`，对外暴露唯一 `Module() dig.Option`，封装领域内部 Provide + 路由 Invoke

### 1.2 轻量化基础设施简化

简单基础设施包（config / pgdb）仅提供单个构造函数、无 Invoke、无子模块：
- **移除**独立 `module.go`，直接暴露顶层公开构造函数
- 根 di.go 行内 `dig.Provide(pkg.Constructor)` 注册
- 复杂基础设施（server，多构造+生命周期 Invoke）保留 `module.go`，通过 `server.Module()` 注册

### 1.3 外部配置两段式（核心）

配置流程严格拆分为两个隔离阶段：

**阶段 1 — 仅 main 处理外部配置 IO**：
- 仅 `main.go` 调用 `config.LoadAppConfig()` 解析命令行参数、环境变量、.env 文件
- `config.LoadAppConfig()` 是纯工具函数，**禁止**在 di.go 中通过 `dig.Provide()` 注册
- 统一使用 Viper 多源覆盖，禁止单独使用 godotenv
- 自定义包装类型（`PGDSN`、`HTTPListenAddr`）解决字符串注入冲突

**阶段 2 — AppConfig 作为 InitApp 顶层入参**：
- InitApp 签名固定：`func InitApp(cfg *config.AppConfig) func(context.Context) error`
- dig 原生特性：顶层形参自动注册为容器全局依赖，**无需** `dig.Supply(cfg)`
- 下游构造仅声明 `cfg *config.AppConfig` 参数，dig 自动注入
- **禁止**任何包级全局 `var cfg *config.AppConfig` 单例

### 1.4 极简命名规则

子文件夹已承载领域标识，禁止重复领域前缀：

| 位置 | ❌ 禁止 | ✅ 规范 |
|------|---------|---------|
| 文件名 | `order_repo.go` | `repo.go` |
| 结构体 | `OrderRepo` | `Repo` |
| 构造函数 | `NewOrderRepo` | `New` |

### 1.5 统一路由注册

每个 Handler 必须定义固定名称的路由注册方法：
```go
func (h *Handler) RegisterRoute(mux *http.ServeMux)
```
领域 `module.go` 的 Invoke 仅调用此方法，禁止在 Invoke 闭包内散落路由代码。

### 1.6 全局注入顺序

根 `dig.Build()` 固定顺序：`dig.Provide(pgdb.NewPGClient)` → 业务领域 `.Module()` → `server.Module()`

## 2. 目录结构标准

```
internal/
  config/                 # 轻量化纯工具包，无 module.go，仅 main 调用
    config.go             # LoadAppConfig()，不参与 DI 注册
    types.go              # 包装类型 + AppConfig 结构体
  pgdb/                   # 轻量化单构造基础设施，无 module.go
    client.go             # 构造接收 dig 自动注入的 *config.AppConfig
  server/                 # 复杂多构造基础设施，保留 module.go
    module.go
    server.go             # NewHTTPServer 接收自动注入的 cfg
    router.go
  domain/                 # 所有垂直业务领域
    user/
      module.go           # 领域强制模块入口
      model/model.go
      repo/repo.go        # 极简文件名
      service/service.go
      handler/handler.go
    order/
      module.go
      model/model.go
      repo/repo.go
      service/service.go
      handler/handler.go
```

## 3. 代码模板

### 3.1 config 包（internal/config）

**types.go**：
```go
package config

import "time"

type PGDSN string
type HTTPListenAddr string

type AppConfig struct {
	PG struct {
		DSN               PGDSN         `mapstructure:"pg_dsn"`
		MaxOpenConns      int           `mapstructure:"pg_max_open"`
		MaxIdleConns      int           `mapstructure:"pg_max_idle"`
		ConnMaxLifetime   time.Duration `mapstructure:"pg_conn_life"`
		EnableAutoMigrate bool          `mapstructure:"pg_auto_migrate"`
	}
	HTTP struct {
		ListenAddr HTTPListenAddr `mapstructure:"http_addr"`
		Timeout    time.Duration  `mapstructure:"http_timeout"`
	}
}
```

**config.go**（仅 main 调用，无全局变量，不参与 DI 注册）：
```go
package config

import (
	"flag"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func LoadAppConfig() (*AppConfig, error) {
	v := viper.New()
	var envFile string
	flag.StringVar(&envFile, "env", ".env", "指定环境配置文件路径")
	flag.Parse()

	v.SetConfigFile(envFile)
	if err := v.ReadInConfig(); err != nil {
		return nil, errors.Wrapf(err, "读取环境文件 %s 失败", envFile)
	}
	v.AutomaticEnv()

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, errors.Wrap(err, "配置反序列化失败")
	}
	return &cfg, nil
}
```

### 3.2 main 入口（外部加载配置，传入 InitApp 顶层形参）

```go
package main

import (
	"context"
	"os"
	"project/internal/config"
)

func main() {
	cfg, err := config.LoadAppConfig()
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := InitApp(cfg)
	if err := start(ctx); err != nil {
		os.Exit(1)
	}
}
```

### 3.3 根 di.go（无 dig.Supply / dig.Provide(config.LoadAppConfig)）

```go
//go:build digen
package main

import (
	"context"
	"github.com/shanjunmei/dig"
	"project/internal/pgdb"
	"project/internal/server"
	"project/internal/domain/user"
	"project/internal/domain/order"
	"project/internal/config"
)

// cfg 来自 main 外部加载，dig 自动全局供给，无需 dig.Supply()
func InitApp(cfg *config.AppConfig) func(context.Context) error {
	return dig.Build(
		dig.Provide(pgdb.NewPGClient),
		user.Module(),
		order.Module(),
		server.Module(),
	)
}
```

### 3.4 pgdb 包（无 module.go，cfg 自动注入）

```go
package pgdb

import (
	"context"
	"errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"project/internal/config"
)

func NewPGClient(cfg *config.AppConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(string(cfg.PG.DSN)), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		return nil, errors.Wrap(err, "打开数据库失败")
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.PG.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.PG.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.PG.ConnMaxLifetime)
	if err := sqlDB.PingContext(context.Background()); err != nil {
		return nil, errors.Wrap(err, "数据库连通性检测失败")
	}
	if cfg.PG.EnableAutoMigrate {
		// db.AutoMigrate(&model.User{})
	}
	return db, nil
}
```

### 3.5 领域分层模板（以 order 为例）

**repo/repo.go**：
```go
package repo

import (
	"gorm.io/gorm"
	"project/internal/domain/order/model"
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(m *model.Model) error { return r.db.Create(m).Error }
```

**service/service.go**：
```go
package service

import (
	"project/internal/domain/order/repo"
	"project/internal/domain/order/model"
)

type Service struct{ repo *repo.Repo }

func New(r *repo.Repo) *Service { return &Service{repo: r} }

func (s *Service) Create(payload *model.Model) error { return s.repo.Create(payload) }
```

**handler/handler.go**（统一 RegisterRoute）：
```go
package handler

import (
	"encoding/json"
	"net/http"
	"project/internal/domain/order/service"
	"project/internal/domain/order/model"
)

type Handler struct{ svc *service.Service }

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoute(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/order/create", h.Create)
	mux.HandleFunc("GET /api/order/detail", h.Detail)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.Model
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = h.svc.Create(&req)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
}
```

### 3.6 领域 module.go

```go
package order

import (
	"net/http"
	"github.com/shanjunmei/dig"
	"project/internal/domain/order/repo"
	"project/internal/domain/order/service"
	"project/internal/domain/order/handler"
)

func Module() dig.Option {
	return dig.Module(
		dig.Provide(repo.New),
		dig.Provide(service.New),
		dig.Provide(handler.New),
		dig.Invoke(func(mux *http.ServeMux, h *handler.Handler) {
			h.RegisterRoute(mux)
		}),
	)
}
```

### 3.7 server 模块（保留 module.go，cfg 自动注入）

```go
package server

import (
	"context"
	"net/http"
	"github.com/shanjunmei/dig"
	"project/internal/config"
)

type HTTPServer struct {
	mux *http.ServeMux
	cfg *config.AppConfig
	srv *http.Server
}

func NewHTTPServer(mux *http.ServeMux, cfg *config.AppConfig) *HTTPServer {
	return &HTTPServer{
		mux: mux,
		cfg: cfg,
		srv: &http.Server{
			Addr:         string(cfg.HTTP.ListenAddr),
			Handler:      mux,
			ReadTimeout:  cfg.HTTP.Timeout,
			WriteTimeout: cfg.HTTP.Timeout,
		},
	}
}

func (s *HTTPServer) Start() error { return s.srv.ListenAndServe() }
func (s *HTTPServer) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }

func Module() dig.Option {
	return dig.Module(
		dig.Provide(http.NewServeMux),
		dig.Provide(NewHTTPServer),
		dig.Invoke(func(srv *HTTPServer) error { return srv.Start() }),
		dig.Invoke(func(ctx context.Context, srv *HTTPServer) error {
			<-ctx.Done()
			if err := srv.Shutdown(ctx); err != nil {
				Logf("服务关闭异常: %v", err)
			}
			return nil
		}),
	)
}
```

### 3.8 Makefile 与 .env 模板

```makefile
digen:
	digen -debug -unused error ./...

run-dev: digen
	go run . --env=.env.dev

build-prod: digen
	CGO_ENABLED=0 go build -o app ./main.go
```

```env
# Postgres
pg_dsn=postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable
pg_max_open=20
pg_max_idle=5
pg_conn_life=1h
pg_auto_migrate=true

# HTTP Server
http_addr=0.0.0.0:8080
http_timeout=30s
```

## 4. 语法约束与检查清单

### 4.1 digen 通用语法约束

1. 闭包不能捕获 InitApp 局部变量，仅允许包级变量/字面量
2. `//go:build digen` 文件仅含 import、InitApp、dig API，禁止业务类型定义
3. 基础类型冲突用包装类型（PGDSN、HTTPListenAddr）
4. 泛型构造必须显式实例化
5. 顶层 Module() 不能用 if 包裹，编译期切换用 build tag
6. InitApp 顶层形参自动 Supply，无需手动 `dig.Supply()`

### 4.2 工业规范检查清单

代码生成故障排查时，按以下清单逐项检查：

**架构与目录**：
- [ ] 根目录是否存在平铺共用 repo/service/handler（禁止跨领域耦合）
- [ ] config/pgdb 是否保留了多余 module.go
- [ ] 根 di.go 是否调用了 `config.Module()`/`pgdb.Module()`（应行内 dig.Provide）
- [ ] 领域子文件夹内文件/结构体/构造是否携带冗余领域前缀
- [ ] 单个业务领域是否导出了多个 Module()

**配置隔离**：
- [ ] 非 config 包是否直接读取命令行/环境变量/Viper 实例
- [ ] 是否存在包级全局 `var cfg *config.AppConfig` 单例
- [ ] di.go 是否手动写了 `dig.Supply(cfg)`（dig 自动捕获，重复供给报错）
- [ ] di.go 是否通过 `dig.Provide()` 注册了 `config.LoadAppConfig()`
- [ ] 构造之间是否手动传递配置实例（应靠 dig 自动注入）

**路由与分层**：
- [ ] 路由逻辑是否散落在领域 Module Invoke 闭包内（应统一用 RegisterRoute）
- [ ] Handler 路由方法名是否统一为 `RegisterRoute(mux *http.ServeMux)`
- [ ] 领域 repo/service 层是否添加了 Invoke（禁止）
- [ ] 领域内部依赖方向是否正确：model ← repo ← service ← handler
- [ ] 业务领域 Module() 内是否聚合了跨领域或基础设施模块
- [ ] PGDB 构造是否省略了 Ping 健康检测

### 4.3 场景输出规范

1. **单领域演示**：输出干净极简领域文件夹 + config 包 Viper 实现 + main 外部加载 + 根 di.go 无 Supply
2. **多领域工业项目**：输出完整垂直多领域目录，config/pgdb 无 module.go，每个 Handler 统一 RegisterRoute
3. **旧项目迁移**：godotenv→Viper、文件重命名去前缀、提取 RegisterRoute、删除 module.go、删除 dig.Supply(cfg)、靠 dig 顶层入参自动注入
4. **完整脚手架**：输出完整可运行工程（目录树+config+main+di.go+pgdb+领域分层+server+Makefile+.env）

## 5. 三方 DI 框架对比

| 特性 | dig | Google Wire | Uber Fx |
|------|-----|-------------|---------|
| 方法 | 编译期代码生成 | 编译期代码生成 | 运行时反射 |
| 配置注入 | InitApp 顶层入参自动供给 | 需手动层层传递 | 易滥用全局单例 |
| 语法风格 | Fx 极简 | 冗长反直觉 | Fx 极简 |
| 维护状态 | ✅ 活跃 | ⚠️ 已归档 | ✅ 活跃 |
