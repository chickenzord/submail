package main

import (
	"fmt"
	"os"

	"github.com/chickenzord/submail/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "submail",
	Short: "Virtual inbox router for AI agents",
	Version: fmt.Sprintf("%s (commit: %s, built: %s)",
		version.Version, version.GitCommit, version.BuildDate),
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
