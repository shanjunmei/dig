package config

import "github.com/shanjunmei/dig/internal/model"

type Config struct {
	OutputFile     string
	UnusedMode     model.UnusedMode
	Debug          bool
	DebugAliases   bool   // 打印每个包的 import alias 映射
	AliasType      string // 例如 "full", "short", "obfuscated", "numeric"
	Paths          []string
	InlineClosures bool   // Phase 3: inline simple closures as IIFE
}
