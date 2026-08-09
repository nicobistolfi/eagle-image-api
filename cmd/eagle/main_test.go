package main

import (
	"strings"
	"testing"

	"github.com/nicobistolfi/eagle-image-api/internal/commands"
)

// TestBuildInfoIsForwarded checks the ldflags-injected version metadata
// reaches the CLI, which is what `eagle --version` prints.
func TestBuildInfoIsForwarded(t *testing.T) {
	t.Cleanup(func() { commands.SetBuildInfo(version, commit, date) })

	commands.SetBuildInfo(version, commit, date)

	got := commands.VersionString()
	for _, want := range []string{version, commit, date} {
		if !strings.Contains(got, want) {
			t.Errorf("VersionString() = %q, want it to contain %q", got, want)
		}
	}
}

// TestDefaultBuildInfo documents the values a `go build` (no ldflags) leaves
// in place, so a broken release pipeline is visible rather than silent.
func TestDefaultBuildInfo(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want dev as the un-injected default", version)
	}
	if commit != "none" {
		t.Errorf("commit = %q, want none as the un-injected default", commit)
	}
	if date != "unknown" {
		t.Errorf("date = %q, want unknown as the un-injected default", date)
	}
}
