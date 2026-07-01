package logger

import (
	"fmt"

	"github.com/shanjunmei/dig/internal/config"
)

type Logger struct {
	Enabled bool
}

func NewLogger(cfg *config.Config) *Logger {
	return &Logger{Enabled: cfg.Debug}
}

func (l *Logger) Debugf(format string, args ...any) {
	if l.Enabled {
		fmt.Printf("[digen] "+format+"\n", args...)
	}
}
