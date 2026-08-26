package closure_capture_string_const

// Config 是被提供的依赖。
type Config struct {
	Font string
}

// NewConfig 构造器。
func NewConfig() *Config { return &Config{} }
