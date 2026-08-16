package main

import (
	"fmt"
	"os"
)

// fail prints msg to stderr and exits 1. It replaces log.Fatal* so user-facing
// errors never carry a log timestamp.
//
// cmd/digen deliberately keeps no package-level mutable state: this is the
// framework's own self-bootstrapping example, so we avoid teaching global
// variables. Flag values are parsed locally in main and passed down explicitly.
//
//go:noreturn
func fail(format string, args ...any) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, args...))
	os.Exit(1)
}
