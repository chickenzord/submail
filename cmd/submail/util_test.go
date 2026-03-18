package main

import (
	"path/filepath"
	"testing"

	"github.com/chickenzord/submail/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── truncate ──────────────────────────────────────────────────────────────────

func TestTruncate_ShortString(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
}

func TestTruncate_ExactLength(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 5))
}

func TestTruncate_LongString(t *testing.T) {
	assert.Equal(t, "hell…", truncate("hello world", 5))
}

func TestTruncate_EmptyString(t *testing.T) {
	assert.Equal(t, "", truncate("", 5))
}

// ── resolveFormat ─────────────────────────────────────────────────────────────
// In test runs stdout is not a TTY, so isTTY() == false.

func TestResolveFormat_QuietWinsOverAll(t *testing.T) {
	assert.Equal(t, fmtQuiet, resolveFormat(true, true))
	assert.Equal(t, fmtQuiet, resolveFormat(false, true))
}

func TestResolveFormat_JSONFlagOrNonTTY(t *testing.T) {
	assert.Equal(t, fmtJSON, resolveFormat(true, false))
	// non-TTY (test runner) also produces JSON even without the flag
	assert.Equal(t, fmtJSON, resolveFormat(false, false))
}

// ── cliErr ────────────────────────────────────────────────────────────────────

func TestCLIErr_Error(t *testing.T) {
	assert.Equal(t, "exit 0", (&cliErr{0}).Error())
	assert.Equal(t, "exit 1", (&cliErr{1}).Error())
	assert.Equal(t, "exit 3", (&cliErr{3}).Error())
}

// ── collectAddresses ──────────────────────────────────────────────────────────

func TestCollectAddresses_Empty(t *testing.T) {
	assert.Nil(t, collectAddresses(nil))
}

func TestCollectAddresses_Single(t *testing.T) {
	agents := []config.AgentConfig{
		{ID: "a", Addresses: []string{"a@example.com"}},
	}
	assert.Equal(t, []string{"a@example.com"}, collectAddresses(agents))
}

func TestCollectAddresses_MultipleAgents(t *testing.T) {
	agents := []config.AgentConfig{
		{ID: "a", Addresses: []string{"a@example.com", "b@example.com"}},
		{ID: "b", Addresses: []string{"c@example.com"}},
	}
	got := collectAddresses(agents)
	assert.ElementsMatch(t, []string{"a@example.com", "b@example.com", "c@example.com"}, got)
}

func TestCollectAddresses_Deduplication(t *testing.T) {
	agents := []config.AgentConfig{
		{ID: "a", Addresses: []string{"shared@example.com", "a@example.com"}},
		{ID: "b", Addresses: []string{"shared@example.com", "b@example.com"}},
	}
	got := collectAddresses(agents)
	assert.Len(t, got, 3)
	assert.ElementsMatch(t, []string{"shared@example.com", "a@example.com", "b@example.com"}, got)
}

// ── resolveConfigPath ─────────────────────────────────────────────────────────

func TestResolveConfigPath_FlagTakesPrecedence(t *testing.T) {
	old := configPath
	configPath = "/explicit/path.yaml"
	t.Cleanup(func() { configPath = old })
	t.Setenv("SUBMAIL_CONFIG", "/env/path.yaml")

	assert.Equal(t, "/explicit/path.yaml", resolveConfigPath())
}

func TestResolveConfigPath_EnvVar(t *testing.T) {
	old := configPath
	configPath = ""
	t.Cleanup(func() { configPath = old })
	t.Setenv("SUBMAIL_CONFIG", "/from/env.yaml")

	assert.Equal(t, "/from/env.yaml", resolveConfigPath())
}

func TestResolveConfigPath_DefaultsToHomeDir(t *testing.T) {
	old := configPath
	configPath = ""
	t.Cleanup(func() { configPath = old })
	t.Setenv("SUBMAIL_CONFIG", "")
	t.Setenv("HOME", "/tmp/fakehome")

	got := resolveConfigPath()
	assert.Equal(t, filepath.Join("/tmp/fakehome", ".config", "submail", "server.yaml"), got)
}

// ── profileFilePath ───────────────────────────────────────────────────────────

func TestProfileFilePath_InvalidNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "foo/bar", "foo\\bar"} {
		_, err := profileFilePath(name)
		require.Error(t, err, "name=%q", name)
		assert.Contains(t, err.Error(), "invalid profile name", "name=%q", name)
	}
}

func TestProfileFilePath_ValidName(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := profileFilePath("myprofile")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/fakehome", ".config", "submail", "profiles", "myprofile.yaml"), got)
}
