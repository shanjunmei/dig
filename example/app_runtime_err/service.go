package app_runtime_err

// Service 是一个简单的服务类型。
type Service struct {
	Name string
}

// NewService 是一个返回 (T, error) 的命名 provider。
// 此处始终成功（返回 nil error），用于验证生成代码中的 panic 路径存在但未被触发。
func NewService() (*Service, error) {
	return &Service{Name: "ok"}, nil
}

// EnhancedService 是闭包 provider 的返回类型，消费跨包 common.Config。
type EnhancedService struct {
	Addr string
	Port int
}
