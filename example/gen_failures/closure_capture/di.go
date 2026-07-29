package gf_closure_capture

import (
	"context"
	"fmt"

	"github.com/shanjunmei/dig"
)

// 场景：闭包捕获 InitApp 的局部变量
// 预期错误：cannot capture local variable "cfg" defined in InitApp scope
func InitClosureCapture(cfg *struct{ Port int }) func(context.Context) error {
	return dig.Build(
		dig.Module(
			// 闭包捕获了 cfg —— 这是被禁止的
			dig.Provide(func() (port int, err error) {
				if cfg.Port < 1024 {
					return 0, fmt.Errorf("port too low")
				}
				return cfg.Port, nil
			}),
			dig.Invoke(func(port int) {}),
		),
	)
}
