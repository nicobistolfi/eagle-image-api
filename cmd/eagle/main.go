package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zantez/image-api/internal/commands"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var showVersion bool

var rootCmd = &cobra.Command{
	Use:   "eagle",
	Short: "Eagle - Image Optimization API CLI",
	Long:  "Eagle CLI allows you to deploy the Eagle Image Optimization API to AWS with a single command.",
	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			fmt.Printf("eagle %s (commit: %s, built: %s)\n", version, commit, date)
			return
		}
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version, commit, and build date")
	rootCmd.AddCommand(commands.DeployCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
