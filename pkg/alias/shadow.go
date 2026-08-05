package alias

import "fmt"

// goBuiltins 是 Go 预声明标识符，生成代码中的变量名不应与之冲突。
// https://go.dev/ref/spec#Predeclared_identifiers
var goBuiltins = []string{
	// 零值与常量
	"nil", "true", "false", "iota",
	// 内建函数
	"append", "cap", "close", "complex", "copy", "delete",
	"imag", "len", "make", "new", "panic", "print", "println",
	"real", "recover",
	// 内建类型（避免与常用变量名冲突的概率较低，但保持完整性）
	"bool", "byte", "rune", "int", "int8", "int16", "int32", "int64",
	"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
	"float32", "float64", "complex64", "complex128",
	"string", "error", "any",
}

// ShadowGuard 防止 digen 生成的变量名遮蔽包标识符或 Go 内建标识符。
// 在每个变量引入点（err、vN、自由变量参数名等）统一查询，避免碎片化补丁。
type ShadowGuard struct {
	reserved map[string]bool
}

// NewShadowGuard 从若干别名 map 构建保留名集合，并预填充 Go 内建标识符。
// 传入 importAliasMap、pkgAliasMap、pkgNameMap 即可覆盖所有包标识符来源。
func NewShadowGuard(maps ...map[string]string) *ShadowGuard {
	sg := &ShadowGuard{reserved: make(map[string]bool)}
	for _, m := range maps {
		for _, name := range m {
			if name != "" {
				sg.reserved[name] = true
			}
		}
	}
	for _, b := range goBuiltins {
		sg.reserved[b] = true
	}
	return sg
}

// SafeName 返回不与保留名冲突的名称。
// 若 desired 未被占用则原样返回；否则追加递增数字后缀直到找到可用名。
func (sg *ShadowGuard) SafeName(desired string) string {
	if !sg.reserved[desired] {
		return desired
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", desired, i)
		if !sg.reserved[candidate] {
			return candidate
		}
	}
}

// Reserved 返回保留名集合（只读用途）。
// 供 pickCtxParamName 等已有逻辑复用，避免重复构建 usedAliases。
func (sg *ShadowGuard) Reserved() map[string]bool {
	return sg.reserved
}

// Reserve 将一个名称加入保留集，用于作用域内的局部保留
// （如闭包参数名，防止自由变量参数名与之冲突）。
func (sg *ShadowGuard) Reserve(name string) {
	if name != "" {
		sg.reserved[name] = true
	}
}
