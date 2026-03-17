package main

import (
	"errors"
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
	// Let commands print their own errors; we just forward the exit code.
	SilenceErrors: true,
	SilenceUsage:  true,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ce *cliErr
		if errors.As(err, &ce) {
			os.Exit(ce.code)
		}
		// Unexpected error not wrapped in cliErr — print and exit 1.
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(exitFailure)
	}
}
