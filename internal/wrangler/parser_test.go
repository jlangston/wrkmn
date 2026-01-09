package wrangler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleWranglerToml = `
name = "my-worker"
account_id = "abc123"
compatibility_date = "2024-01-01"
main = "src/index.ts"

[vars]
API_URL = "https://api.example.com"

[[routes]]
pattern = "example.com/*"
zone_name = "example.com"

[env.staging]
account_id = "staging-account"
vars = { API_URL = "https://staging-api.example.com" }

[[env.staging.routes]]
pattern = "staging.example.com/*"
zone_name = "staging.example.com"

[env.preview]
vars = { API_URL = "https://preview-api.example.com", DEBUG = "true" }
`

const minimalWranglerToml = `
name = "simple-worker"
`

func TestParse(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	assert.Equal(t, "my-worker", config.Name)
	assert.Equal(t, "abc123", config.AccountID)
	assert.Equal(t, "2024-01-01", config.CompatibilityDate)
	assert.Equal(t, "src/index.ts", config.Main)
}

func TestParse_MinimalConfig(t *testing.T) {
	config, err := Parse([]byte(minimalWranglerToml))
	require.NoError(t, err)

	assert.Equal(t, "simple-worker", config.Name)
	assert.Empty(t, config.AccountID)
	assert.Empty(t, config.Env)
}

func TestParse_Environments(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	assert.Len(t, config.Env, 2)

	staging := config.Env["staging"]
	assert.Equal(t, "staging-account", staging.AccountID)
	assert.Len(t, staging.Routes, 1)
	assert.Equal(t, "staging.example.com/*", staging.Routes[0].Pattern)

	preview := config.Env["preview"]
	assert.Empty(t, preview.AccountID)
	assert.Contains(t, preview.Vars, "DEBUG")
}

func TestParse_Vars(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	assert.Contains(t, config.Vars, "API_URL")
	assert.Equal(t, "https://api.example.com", config.Vars["API_URL"])
}

func TestParse_Routes(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	require.Len(t, config.Routes, 1)
	assert.Equal(t, "example.com/*", config.Routes[0].Pattern)
	assert.Equal(t, "example.com", config.Routes[0].ZoneName)
}

func TestConfig_GetAccountID(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	// Root/production uses root account_id
	assert.Equal(t, "abc123", config.GetAccountID(""))
	assert.Equal(t, "abc123", config.GetAccountID("production"))

	// staging has its own account_id
	assert.Equal(t, "staging-account", config.GetAccountID("staging"))

	// preview doesn't have account_id, falls back to root
	assert.Equal(t, "abc123", config.GetAccountID("preview"))

	// Non-existent env falls back to root
	assert.Equal(t, "abc123", config.GetAccountID("nonexistent"))
}

func TestConfig_GetEnvironments(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	envs := config.GetEnvironments()

	assert.Contains(t, envs, "production")
	assert.Contains(t, envs, "staging")
	assert.Contains(t, envs, "preview")
	assert.Len(t, envs, 3)
}

func TestConfig_GetEnvironments_NoEnvs(t *testing.T) {
	config, err := Parse([]byte(minimalWranglerToml))
	require.NoError(t, err)

	envs := config.GetEnvironments()

	assert.Len(t, envs, 1)
	assert.Equal(t, "production", envs[0])
}

func TestConfig_ResolveEnvironment_Production(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	resolved := config.ResolveEnvironment("production")

	assert.Equal(t, "production", resolved.Name)
	assert.Equal(t, "abc123", resolved.AccountID)
	assert.Equal(t, "2024-01-01", resolved.CompatibilityDate)
	assert.Equal(t, "https://api.example.com", resolved.Vars["API_URL"])
	assert.Len(t, resolved.Routes, 1)
}

func TestConfig_ResolveEnvironment_Staging(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	resolved := config.ResolveEnvironment("staging")

	assert.Equal(t, "staging", resolved.Name)
	assert.Equal(t, "staging-account", resolved.AccountID)
	assert.Equal(t, "https://staging-api.example.com", resolved.Vars["API_URL"])
	assert.Len(t, resolved.Routes, 1)
	assert.Equal(t, "staging.example.com/*", resolved.Routes[0].Pattern)
}

func TestConfig_ResolveEnvironment_Preview(t *testing.T) {
	config, err := Parse([]byte(sampleWranglerToml))
	require.NoError(t, err)

	resolved := config.ResolveEnvironment("preview")

	assert.Equal(t, "preview", resolved.Name)
	// Falls back to root account_id
	assert.Equal(t, "abc123", resolved.AccountID)
	// Overrides API_URL
	assert.Equal(t, "https://preview-api.example.com", resolved.Vars["API_URL"])
	// Has extra DEBUG var
	assert.Equal(t, "true", resolved.Vars["DEBUG"])
	// Falls back to root routes
	assert.Len(t, resolved.Routes, 1)
	assert.Equal(t, "example.com/*", resolved.Routes[0].Pattern)
}

func TestParseFile(t *testing.T) {
	// Create temp directory with wrangler.toml
	tmpDir := t.TempDir()
	wranglerPath := filepath.Join(tmpDir, "wrangler.toml")

	err := os.WriteFile(wranglerPath, []byte(sampleWranglerToml), 0644)
	require.NoError(t, err)

	config, err := ParseFile(wranglerPath)
	require.NoError(t, err)

	assert.Equal(t, "my-worker", config.Name)
}

func TestLoadFromDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	wranglerPath := filepath.Join(tmpDir, "wrangler.toml")

	err := os.WriteFile(wranglerPath, []byte(sampleWranglerToml), 0644)
	require.NoError(t, err)

	config, err := LoadFromDirectory(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "my-worker", config.Name)
}

func TestLoadFromDirectory_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadFromDirectory(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Initially does not exist
	assert.False(t, Exists(tmpDir))

	// Create wrangler.toml
	wranglerPath := filepath.Join(tmpDir, "wrangler.toml")
	err := os.WriteFile(wranglerPath, []byte(minimalWranglerToml), 0644)
	require.NoError(t, err)

	// Now it exists
	assert.True(t, Exists(tmpDir))
}

func TestGetWorkerName(t *testing.T) {
	config := &Config{Name: "config-name"}
	assert.Equal(t, "config-name", GetWorkerName(config, "/some/path/dir-name"))

	// Falls back to directory name if config name is empty
	config = &Config{}
	assert.Equal(t, "dir-name", GetWorkerName(config, "/some/path/dir-name"))

	// Falls back to directory name if config is nil
	assert.Equal(t, "dir-name", GetWorkerName(nil, "/some/path/dir-name"))
}

func TestParse_InvalidToml(t *testing.T) {
	invalidToml := `this is not valid [toml`
	_, err := Parse([]byte(invalidToml))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing wrangler config")
}
