package app_gen_test

import (
	"fmt"

	"github.com/shanjunmei/dig/example/common"
)

// makeConfigHandler 是命名函数,返回函数类型 func(*common.Config) error。
// 其返回类型在 types 包中是 *types.Signature,Signature.Params() 引用了 common 包。
// 用于测试 collectUsedPkgsFromType 是否能正确收集 *types.Signature 中的跨包引用。
func makeConfigHandler() func(*common.Config) error {
	return func(cfg *common.Config) error {
		fmt.Println("addr:", cfg.Addr)
		return nil
	}
}
