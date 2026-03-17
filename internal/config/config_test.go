package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveSecret ---

func TestResolveSecret_Fallback(t *testing.T) {
	t.Setenv("TEST_SECRET", "")
	t.Setenv("TEST_SECRET__FILE", "")

	got, err := resolveSecret("TEST_SECRET", "fallback-value")
	require.NoError(t, err)
	assert.Equal(t, "fallback-value", got)
}

func TestResolveSecret_EnvVar(t *testing.T) {
	t.Setenv("TEST_SECRET", "from-env")
	t.Setenv("TEST_SECRET__FILE", "")

	got, err := resolveSecret("TEST_SECRET", "fallback-value")
	require.NoError(t, err)
	assert.Equal(t, "from-env", got)
}

func TestResolveSecret_File(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(secretFile, []byte("from-file\n"), 0600))

	t.Setenv("TEST_SECRET", "from-env")
	t.Setenv("TEST_SECRET__FILE", secretFile)

	// __FILE takes precedence over the direct env var
	got, err := resolveSecret("TEST_SECRET", "fallback-value")
	require.NoError(t, err)
	assert.Equal(t, "from-file", got) // trailing newline trimmed
}

func TestResolveSecret_FileMissing(t *testing.T) {
	t.Setenv("TEST_SECRET__FILE", "/nonexistent/path/secret.txt")

	_, err := resolveSecret("TEST_SECRET", "fallback")
	assert.ErrorContains(t, err, "read secret file")
}

func TestResolveSecret_FilePrecedenceOverEnv(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(secretFile, []byte("file-wins"), 0600))

	t.Setenv("TEST_SECRET", "env-value")
	t.Setenv("TEST_SECRET__FILE", secretFile)

	got, err := resolveSecret("TEST_SECRET", "fallback")
	require.NoError(t, err)
	assert.Equal(t, "file-wins", got)
}

// --- validate ---

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{ID: "agent1", Token: "tok1"},
			{ID: "Agent2", Token: "tok2"},
			{ID: "ABC123", Token: "tok3"},
		},
	}
	assert.NoError(t, cfg.validate())
}

func TestValidate_EmptyID(t *testing.T) {
	cfg := &Config{Agents: []AgentConfig{{ID: "", Token: "tok"}}}
	assert.ErrorContains(t, cfg.validate(), "id is required")
}

func TestValidate_NonAlphanumericID(t *testing.T) {
	tests := []string{"my-agent", "agent_1", "agent 1", "agent@1"}
	for _, id := range tests {
		cfg := &Config{Agents: []AgentConfig{{ID: id, Token: "tok"}}}
		assert.ErrorContains(t, cfg.validate(), "must be alphanumeric", "id=%q", id)
	}
}

func TestValidate_DuplicateID(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{ID: "agent1", Token: "tok1"},
			{ID: "agent1", Token: "tok2"},
		},
	}
	assert.ErrorContains(t, cfg.validate(), "duplicate id")
}

// --- setDefaults ---

func TestSetDefaults_Applied(t *testing.T) {
	cfg := &Config{}
	cfg.setDefaults()

	assert.Equal(t, ":8080", cfg.Server.Addr)
	assert.Equal(t, "submail.db", cfg.Storage.Path)
	assert.Equal(t, 993, cfg.IMAP.Port)
}

func TestSetDefaults_DoesNotOverrideExisting(t *testing.T) {
	cfg := &Config{
		Server:  ServerConfig{Addr: ":9000"},
		Storage: StorageConfig{Path: "/data/custom.db"},
		IMAP:    IMAPConfig{Port: 143},
	}
	cfg.setDefaults()

	assert.Equal(t, ":9000", cfg.Server.Addr)
	assert.Equal(t, "/data/custom.db", cfg.Storage.Path)
	assert.Equal(t, 143, cfg.IMAP.Port)
}

// --- Load ---

func TestLoad_ValidFile(t *testing.T) {
	cfg, err := Load("testdata/valid.yaml")
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.Server.Addr)
	assert.Equal(t, "imap.test.example.com", cfg.IMAP.Host)
	assert.Equal(t, 993, cfg.IMAP.Port)
	assert.Equal(t, "test@example.com", cfg.IMAP.Username)
	assert.Equal(t, "testpassword", cfg.IMAP.Password)
	assert.Equal(t, "/tmp/submail-test.db", cfg.Storage.Path)
	require.Len(t, cfg.Agents, 1)
	assert.Equal(t, "testagent", cfg.Agents[0].ID)
	assert.Equal(t, "test-token-123", cfg.Agents[0].Token)
	assert.Equal(t, []string{"test+testagent@example.com"}, cfg.Agents[0].Addresses)
}

func TestLoad_AppliesDefaults(t *testing.T) {
	cfg, err := Load("testdata/no-defaults.yaml")
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.Server.Addr)
	assert.Equal(t, "submail.db", cfg.Storage.Path)
	assert.Equal(t, 993, cfg.IMAP.Port)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("testdata/nonexistent.yaml")
	assert.ErrorContains(t, err, "read config")
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(f, []byte(":\tinvalid:\t:::yaml"), 0644))

	_, err := Load(f)
	assert.ErrorContains(t, err, "parse config")
}

func TestLoad_EnvOverridesSecret(t *testing.T) {
	t.Setenv("SUBMAIL_IMAP_PASSWORD", "env-password")

	cfg, err := Load("testdata/valid.yaml")
	require.NoError(t, err)
	assert.Equal(t, "env-password", cfg.IMAP.Password)
}

func TestLoad_AgentTokenFromEnv(t *testing.T) {
	t.Setenv("SUBMAIL_AGENT_TESTAGENT_TOKEN", "env-agent-token")

	cfg, err := Load("testdata/valid.yaml")
	require.NoError(t, err)
	assert.Equal(t, "env-agent-token", cfg.Agents[0].Token)
}
