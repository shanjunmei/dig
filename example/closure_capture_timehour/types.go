package closure_capture_timehour

import "time"

// Config 是被提供的依赖。
type Config struct{ Interval time.Duration }

// NewConfig 构造器。
func NewConfig() *Config { return &Config{} }
