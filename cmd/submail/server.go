package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/imap"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// collectAddresses returns a flat, deduplicated, lower-cased list of all
// email addresses configured across all agents.
func collectAddresses(agents []config.AgentConfig) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, a := range agents {
		for _, addr := range a.Addresses {
			if _, ok := seen[addr]; !ok {
				seen[addr] = struct{}{}
				out = append(out, addr)
			}
		}
	}
	return out
}

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

	// Cancelled when SIGINT or SIGTERM is received.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)

	// ── API server ────────────────────────────────────────────────────────────
	srv := api.NewServer(cfg, store)

	g.Go(func() error {
		log.Printf("starting API server on %s", cfg.Server.Addr)
		if err := srv.Start(cfg.Server.Addr); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	// Shut the API server down once the group context is cancelled.
	g.Go(func() error {
		<-gCtx.Done()
		slog.Info("shutting down API server")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	})

	// ── IMAP ingester ─────────────────────────────────────────────────────────
	// Collect all agent addresses into a flat, deduplicated list. This is the
	// whitelist passed to the ingester so it only fetches relevant mail.
	agentAddrs := collectAddresses(cfg.Agents)
	ingester := imap.New(cfg.IMAP, agentAddrs, store)

	g.Go(func() error {
		slog.Info("starting IMAP ingester",
			"host", cfg.IMAP.Host,
			"mailbox", cfg.IMAP.Mailbox,
			"poll_interval", cfg.IMAP.PollInterval,
		)
		return ingester.Run(gCtx)
	})

	return g.Wait()
}
