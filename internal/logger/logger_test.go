package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"silly", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"Info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"error", slog.LevelError},
		{" error ", slog.LevelError},
		{"", slog.LevelDebug},
		{"nonsense", slog.LevelDebug},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseLevel(tt.in); got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestInitWithWriterFiltersBelowLevel(t *testing.T) {
	var buf bytes.Buffer
	InitWithWriter(&buf, "warn")

	Debug("debug message")
	Info("info message")
	Error("error message")

	out := buf.String()
	if strings.Contains(out, "debug message") {
		t.Error("debug record should be filtered out at warn level")
	}
	if strings.Contains(out, "info message") {
		t.Error("info record should be filtered out at warn level")
	}
	if !strings.Contains(out, "error message") {
		t.Errorf("error record should be emitted at warn level, got %q", out)
	}
}

func TestInitWithWriterEmitsAttributes(t *testing.T) {
	var buf bytes.Buffer
	InitWithWriter(&buf, "debug")

	Debug("fetching", "url", "https://example.com/a.jpg")
	Info("processed", "bytes", 1024)

	out := buf.String()
	for _, want := range []string{
		`msg=fetching`,
		`url=https://example.com/a.jpg`,
		`msg=processed`,
		`bytes=1024`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got %q", want, out)
		}
	}
}

func TestInitUsesStdout(t *testing.T) {
	// Init is a thin wrapper over InitWithWriter; exercise it so the default
	// wiring is covered and cannot silently break.
	Init("info")

	ctx := context.Background()
	if !slog.Default().Enabled(ctx, slog.LevelInfo) {
		t.Error(`expected info level to be enabled after Init("info")`)
	}
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		t.Error(`expected debug level to be disabled after Init("info")`)
	}
}
