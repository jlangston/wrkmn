package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvFile(t *testing.T) {
	content := `
# Database config
DATABASE_URL=postgresql://localhost:5432/db
API_KEY="secret-key-123"
QUOTED='single quotes'
export EXPORTED_VAR=exported

EMPTY=
SPACES_IN_VALUE=hello world
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".dev.vars")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	env, err := ParseEnvFile(path)
	require.NoError(t, err)

	tests := map[string]string{
		"DATABASE_URL":    "postgresql://localhost:5432/db",
		"API_KEY":         "secret-key-123",
		"QUOTED":          "single quotes",
		"EXPORTED_VAR":    "exported",
		"EMPTY":           "",
		"SPACES_IN_VALUE": "hello world",
	}

	for key, want := range tests {
		assert.Equal(t, want, env[key], "key %s", key)
	}
}

func TestParseEnvFile_Comments(t *testing.T) {
	content := `# Full line comment
KEY1=value1
# Another comment
KEY2=value2
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	env, err := ParseEnvFile(path)
	require.NoError(t, err)

	assert.Equal(t, "value1", env["KEY1"])
	assert.Equal(t, "value2", env["KEY2"])
	assert.Len(t, env, 2)
}

func TestParseEnvFile_InvalidLines(t *testing.T) {
	content := `VALID=yes
invalid line without equals
ALSO_VALID=yes
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	env, err := ParseEnvFile(path)
	require.NoError(t, err)

	assert.Equal(t, "yes", env["VALID"])
	assert.Equal(t, "yes", env["ALSO_VALID"])
	assert.Len(t, env, 2)
}

func TestParseEnvFile_QuotedValues(t *testing.T) {
	content := `
DOUBLE="double quoted"
SINGLE='single quoted'
MIXED="value with 'inner' quotes"
PARTIAL="no closing quote
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	env, err := ParseEnvFile(path)
	require.NoError(t, err)

	assert.Equal(t, "double quoted", env["DOUBLE"])
	assert.Equal(t, "single quoted", env["SINGLE"])
	assert.Equal(t, "value with 'inner' quotes", env["MIXED"])
	// Partial quote isn't stripped (mismatched quotes)
	assert.Equal(t, `"no closing quote`, env["PARTIAL"])
}

func TestParseEnvFile_NotFound(t *testing.T) {
	_, err := ParseEnvFile("/nonexistent/path/.env")
	assert.Error(t, err)
}

func TestResolveEnvFiles_Priority(t *testing.T) {
	dir := t.TempDir()

	// Create root .dev.vars (defaults.env_file)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".dev.vars"),
		[]byte("DB=root\nROOT_ONLY=yes\nSHARED=from_root"),
		0644,
	))

	// Create service directory and its .dev.vars
	svcDir := filepath.Join(dir, "service")
	require.NoError(t, os.Mkdir(svcDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(svcDir, ".dev.vars"),
		[]byte("DB=service\nSVC_ONLY=yes\nSHARED=from_service"),
		0644,
	))

	proc := &Process{
		WorkingDir:  "service",
		EnvFile:     ".dev.vars",
		Environment: map[string]string{"DB": "inline", "INLINE_ONLY": "yes"},
	}
	defaults := ProcessDefaults{
		EnvFile:     ".dev.vars",
		Environment: map[string]string{"SHARED": "from_defaults_env"},
	}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	// Inline should win over everything
	assert.Equal(t, "inline", env["DB"])
	assert.Equal(t, "yes", env["INLINE_ONLY"])

	// Service file overrides defaults
	assert.Equal(t, "from_service", env["SHARED"])
	assert.Equal(t, "yes", env["SVC_ONLY"])

	// Root-only should be present
	assert.Equal(t, "yes", env["ROOT_ONLY"])
}

func TestResolveEnvFiles_EnvFilesArray(t *testing.T) {
	dir := t.TempDir()

	// Create multiple env files
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "base.env"),
		[]byte("A=base\nB=base\nC=base"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "override.env"),
		[]byte("B=override\nC=override"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "final.env"),
		[]byte("C=final"),
		0644,
	))

	proc := &Process{
		EnvFiles: []string{"base.env", "override.env", "final.env"},
	}
	defaults := ProcessDefaults{}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	// Files should be merged in array order
	assert.Equal(t, "base", env["A"])
	assert.Equal(t, "override", env["B"])
	assert.Equal(t, "final", env["C"])
}

func TestResolveEnvFiles_MissingFilesIgnored(t *testing.T) {
	dir := t.TempDir()

	proc := &Process{
		EnvFile:     "nonexistent.env",
		Environment: map[string]string{"INLINE": "yes"},
	}
	defaults := ProcessDefaults{
		EnvFile: "also-missing.env",
	}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	// Should still have inline vars even if files are missing
	assert.Equal(t, "yes", env["INLINE"])
}

func TestResolveEnvFiles_AbsolutePaths(t *testing.T) {
	dir := t.TempDir()

	envPath := filepath.Join(dir, "absolute.env")
	require.NoError(t, os.WriteFile(envPath, []byte("KEY=absolute"), 0644))

	proc := &Process{
		EnvFile: envPath, // Absolute path
	}
	defaults := ProcessDefaults{}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	assert.Equal(t, "absolute", env["KEY"])
}

func TestResolveEnvFiles_WorkingDirRelative(t *testing.T) {
	dir := t.TempDir()

	// Create nested structure
	nested := filepath.Join(dir, "apps", "worker")
	require.NoError(t, os.MkdirAll(nested, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, ".env"),
		[]byte("NESTED=yes"),
		0644,
	))

	proc := &Process{
		WorkingDir: "apps/worker",
		EnvFile:    ".env",
	}
	defaults := ProcessDefaults{}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	assert.Equal(t, "yes", env["NESTED"])
}

func TestResolveEnvFiles_ParentDirReference(t *testing.T) {
	dir := t.TempDir()

	// Root env file
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "shared.env"),
		[]byte("SHARED=yes"),
		0644,
	))

	// Service directory
	svcDir := filepath.Join(dir, "service")
	require.NoError(t, os.Mkdir(svcDir, 0755))

	proc := &Process{
		WorkingDir: "service",
		EnvFiles:   []string{"../shared.env"}, // Reference parent
	}
	defaults := ProcessDefaults{}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	assert.Equal(t, "yes", env["SHARED"])
}

func TestResolveEnvFiles_EmptyProcess(t *testing.T) {
	dir := t.TempDir()

	proc := &Process{}
	defaults := ProcessDefaults{}

	env, err := ResolveEnvFiles(proc, defaults, dir)
	require.NoError(t, err)

	assert.Empty(t, env)
}
