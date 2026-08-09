// Package logger configures the application-wide structured logger.
//
// Init installs a slog text handler on the standard library's default logger,
// so any package that calls slog directly shares the same configuration. The
// Debug, Info, and Error helpers are thin wrappers kept for convenience and to
// keep call sites free of a slog import.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// ParseLevel maps a human-readable level name to a [slog.Level].
//
// Recognised names are "silly", "debug", "info", "warn", "warning", and
// "error", matched case-insensitively. Any unrecognised value — including the
// empty string — falls back to [slog.LevelDebug], which favours visibility
// over silence when the level is misconfigured.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "silly", "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}

// Init configures the default slog logger to write text records to stdout at
// the level named by level. See [ParseLevel] for the accepted names.
func Init(level string) {
	InitWithWriter(os.Stdout, level)
}

// InitWithWriter behaves like [Init] but sends records to w. It exists so
// tests and embedders can capture log output.
func InitWithWriter(w io.Writer, level string) {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: ParseLevel(level),
	})
	slog.SetDefault(slog.New(handler))
}

// Debug logs msg at debug level with the given key/value pairs.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs msg at info level with the given key/value pairs.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Error logs msg at error level with the given key/value pairs.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}
