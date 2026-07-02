package common

type Config struct {
	Addr string
	Port int
}

func NewConfig() *Config {
	return &Config{Addr: "0.0.0.0", Port: 8080}
}
