package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var (
	clientURL   string
	clientToken string
)

var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Interact with your virtual inbox",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		return resolveClientFlags()
	},
}

var inboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages in the inbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		return errors.New("not implemented")
	},
}

var inboxGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a message by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement
		return errors.New("not implemented")
	},
}

func init() {
	inboxCmd.PersistentFlags().StringVarP(&clientURL, "url", "u", "",
		"Submail server URL (env: SUBMAIL_URL)")
	inboxCmd.PersistentFlags().StringVarP(&clientToken, "token", "t", "",
		"API token (env: SUBMAIL_TOKEN)")

	inboxCmd.AddCommand(inboxListCmd)
	inboxCmd.AddCommand(inboxGetCmd)
	rootCmd.AddCommand(inboxCmd)
}

// resolveClientFlags fills in clientURL and clientToken from env vars when
// the flags were not explicitly provided.
func resolveClientFlags() error {
	if clientURL == "" {
		clientURL = os.Getenv("SUBMAIL_URL")
	}
	if clientToken == "" {
		clientToken = os.Getenv("SUBMAIL_TOKEN")
	}
	if clientURL == "" {
		return errors.New("server URL is required (--url or SUBMAIL_URL)")
	}
	if clientToken == "" {
		return errors.New("API token is required (--token or SUBMAIL_TOKEN)")
	}
	return nil
}
