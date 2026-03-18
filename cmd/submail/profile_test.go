package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateHome sets HOME to a fresh temp dir so profile file I/O never touches
// the real ~/.config/submail directory.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// ── loadProfile ───────────────────────────────────────────────────────────────

func TestLoadProfile_DefaultMissingReturnsNil(t *testing.T) {
	isolateHome(t)
	cfg, err := loadProfile("default")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestLoadProfile_NamedMissingReturnsError(t *testing.T) {
	isolateHome(t)
	_, err := loadProfile("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ── saveProfile / loadProfile roundtrip ──────────────────────────────────────

func TestSaveAndLoadProfile_Roundtrip(t *testing.T) {
	isolateHome(t)

	want := &profileConfig{URL: "http://localhost:8080", Token: "secret-token"}
	require.NoError(t, saveProfile("myprofile", want))

	got, err := loadProfile("myprofile")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want.URL, got.URL)
	assert.Equal(t, want.Token, got.Token)
}

func TestSaveProfile_CreatesDirectories(t *testing.T) {
	home := isolateHome(t)
	// Profile dir should not exist yet.
	dir := filepath.Join(home, ".config", "submail", "profiles")
	_, err := os.Stat(dir)
	require.True(t, os.IsNotExist(err))

	require.NoError(t, saveProfile("new", &profileConfig{URL: "http://x", Token: "t"}))

	_, err = os.Stat(dir)
	assert.NoError(t, err, "profile directory should have been created")
}

func TestSaveProfile_FileIsRestrictedPermissions(t *testing.T) {
	isolateHome(t)
	require.NoError(t, saveProfile("secret", &profileConfig{URL: "http://x", Token: "tok"}))

	path, err := profileFilePath("secret")
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// ── loadProfile with invalid YAML ─────────────────────────────────────────────

func TestLoadProfile_InvalidYAML(t *testing.T) {
	home := isolateHome(t)
	dir := filepath.Join(home, ".config", "submail", "profiles")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{invalid:\t[yaml"), 0o600))

	_, err := loadProfile("bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse profile")
}

// ── listProfiles ──────────────────────────────────────────────────────────────

func TestListProfiles_DirMissingReturnsEmpty(t *testing.T) {
	isolateHome(t)
	names, err := listProfiles()
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestListProfiles_ReturnsSavedProfiles(t *testing.T) {
	isolateHome(t)
	require.NoError(t, saveProfile("alpha", &profileConfig{URL: "http://a", Token: "ta"}))
	require.NoError(t, saveProfile("beta", &profileConfig{URL: "http://b", Token: "tb"}))

	names, err := listProfiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alpha", "beta"}, names)
}

func TestListProfiles_IgnoresNonYAMLFiles(t *testing.T) {
	home := isolateHome(t)
	dir := filepath.Join(home, ".config", "submail", "profiles")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	// A .yaml profile and a non-.yaml file.
	require.NoError(t, saveProfile("real", &profileConfig{URL: "http://x", Token: "t"}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o700))

	names, err := listProfiles()
	require.NoError(t, err)
	assert.Equal(t, []string{"real"}, names)
}
