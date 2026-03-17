package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var alphanumericRE = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	IMAP    IMAPConfig    `yaml:"imap"`
	Storage StorageConfig `yaml:"storage"`
	Agents  []AgentConfig `yaml:"agents"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// TLSMode controls how the IMAP connection is secured.
// Valid values: "tls" (implicit TLS, default), "starttls", "none".
type TLSMode string

const (
	TLSModeImplicit  TLSMode = "tls"
	TLSModeSTARTTLS  TLSMode = "starttls"
	TLSModeNone      TLSMode = "none"
)

type IMAPConfig struct {
	Host     string  `yaml:"host"`
	Port     int     `yaml:"port"`
	Username string  `yaml:"username"`
	Password string  `yaml:"password"`
	Mailbox  string  `yaml:"mailbox"`
	TLSMode  TLSMode `yaml:"tls_mode"`
}

type StorageConfig struct {
	Path string `yaml:"path"`
}

type AgentConfig struct {
	ID        string   `yaml:"id"`
	Token     string   `yaml:"token"`
	Addresses []string `yaml:"addresses"`
}

func (c *Config) setDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Storage.Path == "" {
		c.Storage.Path = "submail.db"
	}
	if c.IMAP.Port == 0 {
		c.IMAP.Port = 993
	}
	if c.IMAP.Mailbox == "" {
		c.IMAP.Mailbox = "INBOX"
	}
	if c.IMAP.TLSMode == "" {
		c.IMAP.TLSMode = TLSModeImplicit
	}
}

func (c *Config) validate() error {
	if c.IMAP.Host == "" {
		return fmt.Errorf("imap.host is required")
	}
	switch c.IMAP.TLSMode {
	case TLSModeImplicit, TLSModeSTARTTLS, TLSModeNone:
		// valid
	default:
		return fmt.Errorf("imap.tls_mode %q is invalid: must be one of tls, starttls, none", c.IMAP.TLSMode)
	}

	seen := make(map[string]bool)
	for i, a := range c.Agents {
		if a.ID == "" {
			return fmt.Errorf("agents[%d]: id is required", i)
		}
		if !alphanumericRE.MatchString(a.ID) {
			return fmt.Errorf("agents[%d]: id %q must be alphanumeric", i, a.ID)
		}
		if seen[a.ID] {
			return fmt.Errorf("agents[%d]: duplicate id %q", i, a.ID)
		}
		seen[a.ID] = true
	}
	return nil
}

// resolveSecrets replaces sensitive fields with values from env vars when present.
// For each field the lookup order is:
//  1. <envVar>__FILE — read secret from the file at that path
//  2. <envVar>       — use the env var value directly
//  3. fallback       — the value already set in the struct (e.g. from YAML)
func (c *Config) resolveSecrets() error {
	var err error

	c.IMAP.Password, err = resolveSecret("SUBMAIL_IMAP_PASSWORD", c.IMAP.Password)
	if err != nil {
		return fmt.Errorf("imap.password: %w", err)
	}

	for i := range c.Agents {
		envVar := "SUBMAIL_AGENT_" + strings.ToUpper(c.Agents[i].ID) + "_TOKEN"
		c.Agents[i].Token, err = resolveSecret(envVar, c.Agents[i].Token)
		if err != nil {
			return fmt.Errorf("agents[%d].token: %w", i, err)
		}
	}

	return nil
}

// resolveSecret resolves a sensitive value using the following precedence:
//  1. env var "<name>__FILE" — read from the file at that path (whitespace trimmed)
//  2. env var "<name>"       — use directly
//  3. fallback               — return as-is
func resolveSecret(name, fallback string) (string, error) {
	if path := os.Getenv(name + "__FILE"); path != "" {
		data, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			return "", fmt.Errorf("read secret file ($%s__FILE): %w", name, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if v := os.Getenv(name); v != "" {
		return v, nil
	}
	return fallback, nil
}

// Load reads and parses a YAML config file, applies defaults, resolves secrets,
// and validates the result.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.setDefaults()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if err := cfg.resolveSecrets(); err != nil {
		return nil, fmt.Errorf("resolve secrets: %w", err)
	}

	return &cfg, nil
}
