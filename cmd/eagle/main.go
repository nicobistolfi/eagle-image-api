// Command eagle deploys the Eagle Image Optimization API to AWS.
//
// Usage:
//
//	eagle deploy --stage prod --region us-west-1
//
// Run `eagle deploy --help` for the full list of deployment parameters.
package main

import (
	"fmt"
	"os"

	"github.com/nicobistolfi/eagle-image-api/internal/commands"
)

// Build metadata, injected at release time via -ldflags -X.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	commands.SetBuildInfo(version, commit, date)

	if err := commands.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
