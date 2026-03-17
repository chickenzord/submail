package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	Addr  string      `yaml:"addr"`
	Admin AdminConfig `yaml:"admin"`
}

// AdminConfig controls the optional web-based admin viewer.
type AdminConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Password string `yaml:"password"` // or use env: SUBMAIL_ADMIN_PASSWORD[__FILE]
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
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	Username     string        `yaml:"username"`
	Password     string        `yaml:"password"`
	Mailbox      string        `yaml:"mailbox"`
	TLSMode      TLSMode       `yaml:"tls_mode"`
	PollInterval time.Duration `yaml:"poll_interval"`
	MaxMailAge   time.Duration `yaml:"max_mail_age"`
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
	if c.IMAP.PollInterval <= 0 {
		c.IMAP.PollInterval = 60 * time.Second
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

	if c.Server.Admin.Enabled && c.Server.Admin.Password == "" {
		return fmt.Errorf("server.admin.password is required when server.admin.enabled is true")
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

// applyEnvOverrides replaces non-sensitive config fields with values from env
// vars when present. This is called after setDefaults so env vars take
// precedence over both YAML values and built-in defaults.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("SUBMAIL_SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("SUBMAIL_ADMIN_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Server.Admin.Enabled = b
		}
	}
	if v := os.Getenv("SUBMAIL_STORAGE_PATH"); v != "" {
		c.Storage.Path = v
	}
	if v := os.Getenv("SUBMAIL_IMAP_HOST"); v != "" {
		c.IMAP.Host = v
	}
	if v := os.Getenv("SUBMAIL_IMAP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.IMAP.Port = n
		}
	}
	if v := os.Getenv("SUBMAIL_IMAP_USERNAME"); v != "" {
		c.IMAP.Username = v
	}
	if v := os.Getenv("SUBMAIL_IMAP_MAILBOX"); v != "" {
		c.IMAP.Mailbox = v
	}
	if v := os.Getenv("SUBMAIL_IMAP_TLS_MODE"); v != "" {
		c.IMAP.TLSMode = TLSMode(v)
	}
	if v := os.Getenv("SUBMAIL_IMAP_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.IMAP.PollInterval = d
		}
	}
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

	c.Server.Admin.Password, err = resolveSecret("SUBMAIL_ADMIN_PASSWORD", c.Server.Admin.Password)
	if err != nil {
		return fmt.Errorf("server.admin.password: %w", err)
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
	cfg.applyEnvOverrides()

	if err := cfg.resolveSecrets(); err != nil {
		return nil, fmt.Errorf("resolve secrets: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}
