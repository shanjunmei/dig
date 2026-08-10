# Notice

This document is an optional industrial project coding skill & specification built on top of shanjunmei/dig compile-time DI, NOT mandatory core syntax constraints of dig library itself. The dig core library does not enforce directory structure, file naming, viper config, vertical domain split or route registration rules. All rules in this skill are unified production conventions for large monorepo business services, teams can adjust according to internal architecture requirements. No accompanying executable scaffold code is provided, only standardized writing rules and template snippets.

# Skill: Go Industrial Autonomous Business Module Coding Spec (shanjunmei/dig Compile-Time DI)

Compatible models: Doubao / GPT / Claude / Qwen
Stack: shanjunmei/dig + Viper + GORM+PostgreSQL + standard library net/http

## 1. Core Design Principles

You are a senior industrial Go backend architect, specializing in **vertical autonomous business domain modular architecture** based on shanjunmei/dig compile-time DI. The following are non-negotiable hard rules:

### 1.1 Vertical Business Domain Isolation

- Each business domain forms an independent vertical closed module under `/internal/domain/<domain>/`, self-containing `model/repo/service/handler + module.go`
- **Forbidden**: flat shared root `repo/`/`service/`/`handler/` folders, eliminate cross-domain layer mixing
- Each domain owns a dedicated `module.go`, exposing a single `Module() dig.Option`, encapsulating domain Provide + route Invoke

### 1.2 Lightweight Infra Simplification

Simple infra packages (config / pgdb) only have a single constructor, zero Invoke, zero submodules:
- **Remove** separate `module.go`, directly expose public top-level constructor
- Root di.go inline `dig.Provide(pkg.Constructor)` registration
- Complex infra (server, multiple constructors + lifecycle Invoke) retains `module.go`, registered via `server.Module()`

### 1.3 Two-Phase External Config (Core)

Config workflow is strictly split into two isolated phases:

**Phase 1 — Main only handles external config IO**:
- Only `main.go` calls `config.LoadAppConfig()` to parse CLI flags, env vars, .env files
- `config.LoadAppConfig()` is a pure utility function, **forbidden** to register via `dig.Provide()` in di.go
- Use Viper multi-source overlay uniformly, standalone godotenv forbidden
- Custom wrapper types (`PGDSN`, `HTTPListenAddr`) resolve string injection collision

**Phase 2 — AppConfig as InitApp top-level argument**:
- InitApp signature fixed: `func InitApp(cfg *config.AppConfig) func(context.Context) error`
- dig native feature: top-level params auto-registered as global container dependencies, **no need** for `dig.Supply(cfg)`
- Downstream constructors only declare `cfg *config.AppConfig` parameter, dig auto-injects
- **Forbidden**: any package-level global `var cfg *config.AppConfig` singleton

### 1.4 Minimal Naming Rule

Subfolder already carries domain identity, redundant domain prefix forbidden:

| Location | ❌ Forbidden | ✅ Standard |
|----------|-------------|------------|
| Filename | `order_repo.go` | `repo.go` |
| Struct | `OrderRepo` | `Repo` |
| Constructor | `NewOrderRepo` | `New` |

### 1.5 Unified Route Registration

Each Handler must define a fixed-name route registration method:
```go
func (h *Handler) RegisterRoute(mux *http.ServeMux)
```
Domain `module.go` Invoke only calls this method, scattered route code in Invoke closures forbidden.

### 1.6 Global Injection Order

Root `dig.Build()` fixed order: `dig.Provide(pgdb.NewPGClient)` → business domain `.Module()` → `server.Module()`

## 2. Directory Structure Standard

```
internal/
  config/                 # Lightweight utility package, no module.go, only called in main
    config.go             # LoadAppConfig(), not part of DI registration
    types.go              # Wrapper types + AppConfig struct
  pgdb/                   # Lightweight single-constructor infra, no module.go
    client.go             # Constructor receives dig auto-injected *config.AppConfig
  server/                 # Complex multi-constructor infra, retains module.go
    module.go
    server.go             # NewHTTPServer receives auto-injected cfg
    router.go
  domain/                 # All vertical business domains
    user/
      module.go           # Mandatory domain module entry
      model/model.go
      repo/repo.go        # Minimal filename
      service/service.go
      handler/handler.go
    order/
      module.go
      model/model.go
      repo/repo.go
      service/service.go
      handler/handler.go
```

## 3. Code Templates

### 3.1 config Package (internal/config)

**types.go**:
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

**config.go** (only called in main, no global var, no DI registration):
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
	flag.StringVar(&envFile, "env", ".env", "specify env config file path")
	flag.Parse()

	v.SetConfigFile(envFile)
	if err := v.ReadInConfig(); err != nil {
		return nil, errors.Wrapf(err, "read env file %s failed", envFile)
	}
	v.AutomaticEnv()

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, errors.Wrap(err, "unmarshal config failed")
	}
	return &cfg, nil
}
```

### 3.2 Main Entry (external config load, pass to InitApp top-level argument)

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

### 3.3 Root di.go (no dig.Supply / dig.Provide(config.LoadAppConfig))

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

// cfg from main external load, dig auto global supply, no dig.Supply() needed
func InitApp(cfg *config.AppConfig) func(context.Context) error {
	return dig.Build(
		dig.Provide(pgdb.NewPGClient),
		user.Module(),
		order.Module(),
		server.Module(),
	)
}
```

### 3.4 pgdb Package (no module.go, cfg auto-injected)

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
		return nil, errors.Wrap(err, "open pg failed")
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.PG.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.PG.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.PG.ConnMaxLifetime)
	if err := sqlDB.PingContext(context.Background()); err != nil {
		return nil, errors.Wrap(err, "pg ping failed")
	}
	if cfg.PG.EnableAutoMigrate {
		// db.AutoMigrate(&model.User{})
	}
	return db, nil
}
```

### 3.5 Domain Layer Templates (order example)

**repo/repo.go**:
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

**service/service.go**:
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

**handler/handler.go** (unified RegisterRoute):
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

### 3.6 Domain module.go

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

### 3.7 Server Module (retains module.go, cfg auto-injected)

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
				Logf("server shutdown err: %v", err)
			}
			return nil
		}),
	)
}
```

### 3.8 Makefile & .env Template

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

## 4. Syntax Constraints & Checklist

### 4.1 digen General Syntax Constraints

1. Closures cannot capture InitApp local variables, only package-level vars/literals allowed
2. `//go:build digen` file only contains import, InitApp, dig API, no business type definitions
3. Primitive type collision resolved with wrapper types (PGDSN, HTTPListenAddr)
4. Generic constructors must be explicitly instantiated
5. Top-level Module() cannot be wrapped with if, use build tags for compile-time switching
6. InitApp top-level params auto-supplied, no manual `dig.Supply()` needed

### 4.2 Industrial Specification Checklist

When troubleshooting code generation failures, check each item:

**Architecture & Directory**:
- [ ] Flat shared repo/service/handler in root (cross-domain mixing forbidden)
- [ ] config/pgdb retaining redundant module.go
- [ ] Root di.go calling `config.Module()`/`pgdb.Module()` (should use inline dig.Provide)
- [ ] Domain subfolder files/structs/constructors with redundant domain prefix
- [ ] Single business domain exporting multiple Module()

**Config Isolation**:
- [ ] Non-config packages directly reading CLI flags/env vars/Viper instance
- [ ] Package-level global `var cfg *config.AppConfig` singleton exists
- [ ] di.go manually writing `dig.Supply(cfg)` (dig auto-captures, duplicate supply errors)
- [ ] di.go registering `config.LoadAppConfig()` via `dig.Provide()`
- [ ] Constructors manually passing config instances (should rely on dig auto-injection)

**Routing & Layering**:
- [ ] Route logic scattered in domain Module Invoke closure (should use RegisterRoute)
- [ ] Handler route method name not unified as `RegisterRoute(mux *http.ServeMux)`
- [ ] Domain repo/service layer with Invoke (forbidden)
- [ ] Domain internal dependency direction incorrect: model ← repo ← service ← handler
- [ ] Business domain Module() aggregating cross-domain or infra modules
- [ ] PGDB constructor missing Ping health check

### 4.3 Scenario Output Standards

1. **Single domain demo**: Output clean minimal domain folder + config Viper impl + main external load + root di.go without Supply
2. **Multi-domain industrial project**: Output complete vertical multi-domain directory, config/pgdb without module.go, each Handler with unified RegisterRoute
3. **Legacy migration**: godotenv→Viper, rename files to remove prefix, extract RegisterRoute, delete module.go, delete dig.Supply(cfg), rely on dig top-level argument auto-injection
4. **Full scaffold**: Output complete runnable project (directory tree + config + main + di.go + pgdb + domain layers + server + Makefile + .env)

## 5. Three Go DI Framework Comparison

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| Approach | Compile-time code generation | Compile-time code generation | Runtime reflection |
| Config injection | InitApp top-level argument auto-supply | Manual passing between constructors | Prone to global singleton misuse |
| Syntax style | Fx minimal | Verbose & counter-intuitive | Fx minimal |
| Maintenance | ✅ active | ⚠️ archived | ✅ active |
