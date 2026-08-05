package helper

// Counter 是自由变量 db 的类型。
type Counter struct {
	Count int
}

// Result 是闭包 provider 的返回类型。
type Result struct {
	Value int
}

// db 是包级变量，与主包导入的 example/db 的别名 db 同名。
// 闭包捕获 db 后，提取到主包时参数名若仍为 db 会遮蔽 db 包别名。
var Db = &Counter{Count: 42}
