package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the default slog logger with the given level string.
func Init(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "silly", "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: l,
	})
	slog.SetDefault(slog.New(handler))
}

func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}
