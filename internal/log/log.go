// Package log provides structured logging for go-reachy.
// It wraps slog with sensible defaults for production use.
package log

import (
	"log/slog"
	"os"
	"sync"
)

var (
	logger *slog.Logger
	once   sync.Once
)

// Init initializes the global logger with the specified level.
// Valid levels: "debug", "info", "warn", "error"
func Init(level string) {
	once.Do(func() {
		var lvl slog.Level
		switch level {
		case "debug":
			lvl = slog.LevelDebug
		case "warn":
			lvl = slog.LevelWarn
		case "error":
			lvl = slog.LevelError
		default:
			lvl = slog.LevelInfo
		}

		opts := &slog.HandlerOptions{
			Level: lvl,
		}

		// Use JSON in production, text in development
		if os.Getenv("GO_ENV") == "production" {
			logger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
		} else {
			logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
		}

		slog.SetDefault(logger)
	})
}

// L returns the global logger instance.
func L() *slog.Logger {
	if logger == nil {
		Init("info")
	}
	return logger
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	L().Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	L().Info(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	L().Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	L().Error(msg, args...)
}

// With returns a logger with the given attributes.
func With(args ...any) *slog.Logger {
	return L().With(args...)
}



