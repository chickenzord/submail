package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// profileConfig holds the per-agent connection settings stored in a profile file.
type profileConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// profileDir returns the directory that holds all profile files.
func profileDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "submail"), nil
}

// profileFilePath returns the full path to a named profile file.
// It rejects names that could escape the profile directory.
func profileFilePath(name string) (string, error) {
	if strings.ContainsAny(name, "/\\") || name == ".." || name == "." || name == "" {
		return "", fmt.Errorf("invalid profile name %q", name)
	}
	dir, err := profileDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".yaml"), nil
}

// loadProfile reads a named profile from disk.
// Returns nil, nil when the "default" profile does not exist (silent fallback).
func loadProfile(name string) (*profileConfig, error) {
	path, err := profileFilePath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if name == "default" {
				return nil, nil // missing default is not an error
			}
			return nil, fmt.Errorf("profile %q not found (expected at %s)", name, path)
		}
		return nil, fmt.Errorf("read profile %q: %w", name, err)
	}
	var cfg profileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", name, err)
	}
	return &cfg, nil
}

// saveProfile writes a profile to disk, creating the directory if needed.
func saveProfile(name string, cfg *profileConfig) error {
	path, err := profileFilePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write profile %q: %w", name, err)
	}
	return nil
}

// listProfiles returns the names of all profiles in the config directory.
func listProfiles() ([]string, error) {
	dir, err := profileDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profile directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		// Skip the server config and any non-yaml files.
		if n == "server.yaml" || !strings.HasSuffix(n, ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(n, ".yaml"))
	}
	return names, nil
}

// ── profile command group ──────────────────────────────────────────────────────

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage per-agent connection profiles",
	Long: `Profiles store a server URL and agent token so each invocation does not
require --url and --token flags.

Profile files are stored at ~/.config/submail/<name>.yaml

The active profile is selected (in order of precedence) by:
  1. --profile flag
  2. SUBMAIL_PROFILE environment variable
  3. "default" profile (silent fallback)

Quick start for a multi-agent setup on the same machine:
  submail profile set hawkeye --url http://localhost:8080 --token hawkeye-token
  submail profile set fuery   --url http://localhost:8080 --token fuery-token

  # In the hawkeye agent's environment:
  export SUBMAIL_PROFILE=hawkeye
  submail inbox list          # no --url or --token needed`,
}

// ── profile set ───────────────────────────────────────────────────────────────

var (
	profileSetURL   string
	profileSetToken string
)

var profileSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Create or update a profile",
	Long: `Create or update the profile <name> with the given URL and token.

Examples:
  submail profile set default  --url http://localhost:8080 --token mytoken
  submail profile set hawkeye  --url http://localhost:8080 --token hawkeye-token
  submail profile set fuery    --url http://localhost:8080 --token fuery-token`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]

		// Load existing profile so we can do partial updates.
		existing, err := loadProfile(name)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitFailure}
		}
		if existing == nil {
			existing = &profileConfig{}
		}

		if profileSetURL != "" {
			existing.URL = profileSetURL
		}
		if profileSetToken != "" {
			existing.Token = profileSetToken
		}

		if existing.URL == "" {
			fmt.Fprintln(os.Stderr, "Error: --url is required (or update an existing profile that already has one)")
			return &cliErr{exitUsage}
		}
		if existing.Token == "" {
			fmt.Fprintln(os.Stderr, "Error: --token is required (or update an existing profile that already has one)")
			return &cliErr{exitUsage}
		}

		path, _ := profileFilePath(name)
		if err := saveProfile(name, existing); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitFailure}
		}

		fmt.Printf("Profile %q saved to %s\n", name, path)
		return nil
	},
}

// ── profile list ──────────────────────────────────────────────────────────────

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	Long: `List all profiles in ~/.config/submail/.

Examples:
  submail profile list
  submail profile list --json`,
	RunE: func(_ *cobra.Command, _ []string) error {
		names, err := listProfiles()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitFailure}
		}

		type row struct {
			Name  string `json:"name"`
			URL   string `json:"url"`
			Token string `json:"token_set"`
		}

		rows := make([]row, 0, len(names))
		for _, name := range names {
			cfg, err := loadProfile(name)
			if err != nil || cfg == nil {
				continue
			}
			tokenSet := "no"
			if cfg.Token != "" {
				tokenSet = "yes"
			}
			rows = append(rows, row{Name: name, URL: cfg.URL, Token: tokenSet})
		}

		fmt_ := resolveFormat(jsonFlag, quietFlag)
		switch fmt_ {
		case fmtJSON:
			return jsonEncoder().Encode(rows)
		case fmtQuiet:
			for _, r := range rows {
				fmt.Println(r.Name)
			}
		default:
			if len(rows) == 0 {
				fmt.Println("No profiles found. Use 'submail profile set' to create one.")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PROFILE\tURL\tTOKEN SET")
			fmt.Fprintln(tw, strings.Repeat("─", 12)+"\t"+strings.Repeat("─", 30)+"\t"+strings.Repeat("─", 9))
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.URL, r.Token)
			}
			tw.Flush()
		}
		return nil
	},
}

// ── profile show ──────────────────────────────────────────────────────────────

var profileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile (token is masked)",
	Long: `Display the settings stored in a profile. The token is always masked.

Examples:
  submail profile show hawkeye
  submail profile show hawkeye --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := loadProfile(name)
		if err != nil {
			fmt_ := resolveFormat(jsonFlag, quietFlag)
			if fmt_ == fmtJSON {
				return printJSONError(exitNotFound, "not_found", err.Error(),
					map[string]any{"name": name}, false)
			}
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitNotFound}
		}
		if cfg == nil {
			msg := fmt.Sprintf("profile %q not found", name)
			fmt_ := resolveFormat(jsonFlag, quietFlag)
			if fmt_ == fmtJSON {
				return printJSONError(exitNotFound, "not_found", msg,
					map[string]any{"name": name}, false)
			}
			fmt.Fprintln(os.Stderr, "Error:", msg)
			return &cliErr{exitNotFound}
		}

		masked := "******"
		if cfg.Token == "" {
			masked = "(not set)"
		}

		fmt_ := resolveFormat(jsonFlag, quietFlag)
		switch fmt_ {
		case fmtJSON:
			return jsonEncoder().Encode(map[string]any{
				"name":      name,
				"url":       cfg.URL,
				"token_set": cfg.Token != "",
			})
		case fmtQuiet:
			fmt.Println(name)
		default:
			fmt.Printf("Profile:  %s\n", name)
			fmt.Printf("URL:      %s\n", cfg.URL)
			fmt.Printf("Token:    %s\n", masked)
		}
		return nil
	},
}

// ── profile delete ────────────────────────────────────────────────────────────

var profileDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm"},
	Short:   "Delete a profile",
	Long: `Delete the named profile file.

Examples:
  submail profile delete hawkeye`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		path, err := profileFilePath(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitUsage}
		}
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "Error: profile %q not found\n", name)
				return &cliErr{exitNotFound}
			}
			fmt.Fprintln(os.Stderr, "Error:", err)
			return &cliErr{exitFailure}
		}
		fmt.Printf("Profile %q deleted.\n", name)
		return nil
	},
}

func init() {
	profileSetCmd.Flags().StringVarP(&profileSetURL, "url", "u", "", "Server URL to store in the profile")
	profileSetCmd.Flags().StringVarP(&profileSetToken, "token", "t", "", "Bearer token to store in the profile")

	profileCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false,
		"Output JSON to stdout (auto-enabled when stdout is not a TTY)")
	profileCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false,
		"Output only names, one per line (pipe-friendly)")

	profileCmd.AddCommand(profileSetCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileDeleteCmd)
	rootCmd.AddCommand(profileCmd)
}

// jsonEncoder returns a JSON encoder writing to stdout with no HTML escaping.
func jsonEncoder() *json.Encoder {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc
}
