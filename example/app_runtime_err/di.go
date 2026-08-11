//go:build digen

package app_runtime_err

import (
	"context"
	"fmt"

	"github.com/shanjunmei/dig/example/common"

	"github.com/shanjunmei/dig"
)

// InitRuntimeErr 演示运行时错误传播路径（生成成功，运行时 Invoke 返回 error）：
//
//  1. 命名函数 provider 返回 (T, error) → 生成代码中 panic 路径
//  2. 闭包 provider 消费跨包类型 common.Config，返回 (T, error) → 生成代码中 panic 路径
//  3. Invoke 返回非 nil error → 传播给调用者（InitRuntimeErr()(ctx) 返回 error）
//
// 此示例验证生成代码正确处理 provider error（panic）和 invoke error（return）。
// 生成代码可编译，运行时 Invoke 返回 error，不触发 panic（provider 返回 nil error）。
func InitRuntimeErr() func(context.Context) error {
	return dig.Build(
		dig.Module(
			// 跨包类型 Supply
			dig.Supply(&common.Config{Addr: "localhost", Port: 8080}),

			// 场景1: 命名函数 provider 返回 (T, error)
			// 生成代码: dv0, err := NewService(); if err != nil { panic(err) }
			dig.Provide(NewService),

			// 场景2: 闭包 provider 消费跨包类型，返回 (T, error)
			// 生成代码: dv1, err := dig_provider_N(cfg); if err != nil { panic(err) }
			dig.Provide(func(cfg *common.Config) (enhanced *EnhancedService, err error) {
				if cfg.Port == 0 {
					return nil, fmt.Errorf("port must not be zero")
				}
				return &EnhancedService{Addr: cfg.Addr, Port: cfg.Port}, nil
			}),

			// 场景3: Invoke 返回非 nil error → 传播给调用者
			// 生成代码: if err := dig_invoke_N(svc, enhanced); err != nil { return err }
			dig.Invoke(func(svc *Service, enhanced *EnhancedService) error {
				return fmt.Errorf("invoke intentionally returned error: svc=%s enhanced=%s:%d",
					svc.Name, enhanced.Addr, enhanced.Port)
			}),
		),
	)
}
