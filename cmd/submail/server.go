package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Submail HTTP server",
	RunE:  runServer,
}

var configPath string

func init() {
	serverCmd.Flags().StringVarP(&configPath, "config", "c", "",
		"config file path (env: SUBMAIL_CONFIG, default: ~/.config/submail/server.yaml)")
	rootCmd.AddCommand(serverCmd)
}

func resolveConfigPath() string {
	if configPath != "" {
		return configPath
	}
	if v := os.Getenv("SUBMAIL_CONFIG"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "submail", "server.yaml")
	}
	return "server.yaml"
}

func runServer(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		return err
	}

	store, err := storage.NewSQLiteStore(cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := api.NewServer(cfg, store)

	log.Printf("starting server on %s", cfg.Server.Addr)
	return srv.Start(cfg.Server.Addr)
}
