package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// swapTemplateURL points the template fetch at url for the duration of a test
// and returns a function that restores the original value.
func swapTemplateURL(t *testing.T, url string) func() {
	t.Helper()
	original := templateURL
	templateURL = url
	return func() { templateURL = original }
}

// newDeployCmdForTest builds a deploy command with its own flag storage, so a
// test can set flags without mutating the package-level command.
func newDeployCmdForTest() (*cobra.Command, *deployFlags) {
	dst := &deployFlags{}
	cmd := &cobra.Command{Use: "deploy"}
	registerDeployFlags(cmd, dst)
	return cmd, dst
}

func TestNewRootCmdMetadata(t *testing.T) {
	root := NewRootCmd()

	if root.Use != "eagle" {
		t.Errorf("Use = %q, want eagle", root.Use)
	}
	if root.Short == "" {
		t.Error("Short description should not be empty")
	}
	if root.Flags().Lookup("version") == nil {
		t.Error("expected a --version flag")
	}
	if s := root.Flags().ShorthandLookup("v"); s == nil {
		t.Error("expected a -v shorthand for --version")
	}
}

func TestNewRootCmdHasDeploySubcommand(t *testing.T) {
	root := NewRootCmd()

	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "deploy" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a deploy subcommand")
	}
}

func TestNewRootCmdReturnsFreshCommands(t *testing.T) {
	// Each call must produce an independent command; sharing the --version
	// flag storage between invocations would leak state across runs.
	first := NewRootCmd()
	second := NewRootCmd()

	if first == second {
		t.Fatal("NewRootCmd() returned the same instance twice")
	}
	if err := first.Flags().Set("version", "true"); err != nil {
		t.Fatalf("setting version on the first command: %v", err)
	}
	if second.Flags().Lookup("version").Value.String() != "false" {
		t.Error("setting --version on one command changed another")
	}
}

func TestRootCmdVersionFlag(t *testing.T) {
	SetBuildInfo("1.2.3", "abc1234", "2026-01-01")
	t.Cleanup(func() { SetBuildInfo("dev", "none", "unknown") })

	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	s := out.String()
	for _, want := range []string{"eagle 1.2.3", "abc1234", "2026-01-01"} {
		if !strings.Contains(s, want) {
			t.Errorf("version output missing %q, got %q", want, s)
		}
	}
}

func TestRootCmdWithoutArgsPrintsHelp(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	s := out.String()
	if !strings.Contains(s, "Usage:") {
		t.Errorf("expected help output, got %q", s)
	}
	if !strings.Contains(s, "deploy") {
		t.Errorf("expected help to list the deploy command, got %q", s)
	}
}

func TestVersionStringDefaults(t *testing.T) {
	SetBuildInfo("dev", "none", "unknown")

	got := VersionString()
	if !strings.HasPrefix(got, "eagle dev") {
		t.Errorf("VersionString() = %q, want it to start with %q", got, "eagle dev")
	}
	for _, want := range []string{"commit: none", "built: unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("VersionString() = %q, want it to contain %q", got, want)
		}
	}
}

func TestSetBuildInfo(t *testing.T) {
	t.Cleanup(func() { SetBuildInfo("dev", "none", "unknown") })

	SetBuildInfo("v9.9.9", "deadbeef", "2026-08-08")

	if got := VersionString(); got != "eagle v9.9.9 (commit: deadbeef, built: 2026-08-08)" {
		t.Errorf("VersionString() = %q", got)
	}
}
