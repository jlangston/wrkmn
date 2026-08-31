package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseEnvFile reads a .env/.dev.vars file and returns key-value pairs.
// Supports:
//   - KEY=value
//   - KEY="quoted value"
//   - KEY='single quoted'
//   - # comments
//   - Empty lines (skipped)
//   - Export prefix: export KEY=value
func ParseEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip optional "export " prefix
		line = strings.TrimPrefix(line, "export ")

		// Split on first =
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Remove surrounding quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		env[key] = value
	}

	return env, scanner.Err()
}

// EnvFileRef is a resolved env-file path plus its provenance. Files inherited
// from defaults are best-effort; files a process asked for explicitly must exist.
type EnvFileRef struct {
	Path        string
	FromDefault bool
}

// ResolveEnvFilePaths returns the env files that apply to a process in merge
// order (lowest priority first). Relative paths resolve against baseDir for
// defaults.env_file and against the process working dir otherwise.
func ResolveEnvFilePaths(proc *Process, defaults ProcessDefaults, baseDir string) []EnvFileRef {
	workingDir := proc.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}
	if !filepath.IsAbs(workingDir) {
		workingDir = filepath.Join(baseDir, workingDir)
	}

	var refs []EnvFileRef
	if defaults.EnvFile != "" {
		path := defaults.EnvFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		refs = append(refs, EnvFileRef{Path: path, FromDefault: true})
	}
	for _, file := range proc.EnvFiles {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, file)
		}
		refs = append(refs, EnvFileRef{Path: path})
	}
	if proc.EnvFile != "" {
		path := proc.EnvFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		refs = append(refs, EnvFileRef{Path: path})
	}
	return refs
}

// ResolveEnvFiles loads and merges environment files for a process.
// Merge priority (lowest to highest):
//  1. defaults.env_file — best-effort: a missing file is skipped (it may not
//     apply to every process)
//  2. defaults.environment
//  3. process.env_files (in array order) — missing file is an error
//  4. process.env_file — missing file is an error
//  5. process.environment
func ResolveEnvFiles(proc *Process, defaults ProcessDefaults, baseDir string) (map[string]string, error) {
	env := make(map[string]string)

	var explicit []EnvFileRef
	for _, ref := range ResolveEnvFilePaths(proc, defaults, baseDir) {
		if ref.FromDefault {
			if vars, err := ParseEnvFile(ref.Path); err == nil {
				for k, v := range vars {
					env[k] = v
				}
			}
			continue
		}
		explicit = append(explicit, ref)
	}

	for k, v := range defaults.Environment {
		env[k] = v
	}

	for _, ref := range explicit {
		vars, err := ParseEnvFile(ref.Path)
		if err != nil {
			return nil, fmt.Errorf("env file %s: %w", ref.Path, err)
		}
		for k, v := range vars {
			env[k] = v
		}
	}

	for k, v := range proc.Environment {
		env[k] = v
	}

	return env, nil
}
