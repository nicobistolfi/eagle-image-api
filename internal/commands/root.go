// Package commands implements the subcommands of the `eagle` CLI, which
// deploys the Eagle Image API into a user's own AWS account.
//
// The AWS calls are reached through narrow interfaces ([ecrAPI], [cfnAPI])
// rather than the concrete SDK clients so the deployment logic can be
// exercised without credentials.
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Build metadata, overridden at release time via -ldflags -X.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetBuildInfo overrides the version metadata reported by `eagle --version`.
// It exists for embedders and tests; releases inject the values with -ldflags.
func SetBuildInfo(v, c, d string) {
	version, commit, date = v, c, d
}

// VersionString renders the version line shown by `eagle --version`.
func VersionString() string {
	return fmt.Sprintf("eagle %s (commit: %s, built: %s)", version, commit, date)
}

// NewRootCmd builds the `eagle` root command with all subcommands attached.
// A fresh command is constructed on each call so tests are independent.
func NewRootCmd() *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:           "eagle",
		Short:         "Eagle - Image Optimization API CLI",
		Long:          "Eagle CLI allows you to deploy the Eagle Image Optimization API to AWS with a single command.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), VersionString())
				return nil
			}
			return cmd.Help()
		},
	}

	root.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version, commit, and build date")
	root.AddCommand(DeployCmd)

	return root
}
