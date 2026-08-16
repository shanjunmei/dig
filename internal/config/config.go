package config

import "github.com/shanjunmei/dig/internal/model"

type Config struct {
	OutputFile     string
	UnusedMode     model.UnusedMode
	Debug          bool
	AliasType      string // 例如 "full", "short", "obfuscated", "numeric"
	Paths          []string
	InlineClosures bool // Phase 3: inline simple closures as IIFE
	// TypeCheckNet enables the post-generation type-check safety net that
	// reloads the package (packages.Load) to catch internal generator bugs.
	// It is on by default (safe), but can be disabled for large `./...`
	// runs where reloading the package graph once per generated file is the
	// dominant cost.
	TypeCheckNet bool

	// Cache enables the IR cache: the extractor's output (the stable
	// intermediate representation) is persisted to disk keyed by the package's
	// source hash, so an unchanged package skips the expensive extraction /
	// type-checking step on the next run. Off by default — opt in with -cache.
	Cache bool
	// CacheDir overrides the IR cache directory. Empty means
	// os.TempDir()/digen-ir-cache.
	CacheDir string

	// DryRun makes check-style commands validate the DI contract (extraction +
	// unused-provider check) without emitting any generated file.
	DryRun bool
}
