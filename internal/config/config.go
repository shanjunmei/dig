package config

import "github.com/shanjunmei/dig/internal/model"

type Config struct {
	OutputFile string
	UnusedMode model.UnusedMode
	Debug      bool
	AliasType  string // 例如 "full", "short", "obfuscated", "numeric"
	Paths      []string
}
