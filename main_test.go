package main

import (
	"testing"

	"github.com/nicobistolfi/eagle-image-api/internal/config"
)

// TestSetup covers the startup path main() depends on: configuration is read
// from the environment and libvips comes up cleanly.
func TestSetup(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("QUALITY", "70")

	shutdown, err := setup()
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("setup() returned a nil shutdown function")
	}
	defer shutdown()

	if config.Cfg.LogLevel != "error" {
		t.Errorf("LogLevel = %q, want error", config.Cfg.LogLevel)
	}
	if config.Cfg.Quality != 70 {
		t.Errorf("Quality = %d, want 70", config.Cfg.Quality)
	}
}
