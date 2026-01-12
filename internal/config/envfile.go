package config

import (
	"bufio"
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

// ResolveEnvFiles loads and merges environment files for a process.
// Returns merged environment map with proper priority:
//  1. defaults.env_file (lowest priority)
//  2. defaults.environment
//  3. process.env_files (in array order)
//  4. process.env_file
//  5. process.environment (highest priority)
func ResolveEnvFiles(proc *Process, defaults ProcessDefaults, baseDir string) (map[string]string, error) {
	env := make(map[string]string)

	workingDir := proc.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}
	if !filepath.IsAbs(workingDir) {
		workingDir = filepath.Join(baseDir, workingDir)
	}

	// 1. Load defaults.env_file (relative to baseDir)
	if defaults.EnvFile != "" {
		path := defaults.EnvFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		if vars, err := ParseEnvFile(path); err == nil {
			for k, v := range vars {
				env[k] = v
			}
		}
		// Ignore error if file doesn't exist - it's optional
	}

	// 2. Merge defaults.environment
	for k, v := range defaults.Environment {
		env[k] = v
	}

	// 3. Load process.env_files (relative to working_dir)
	for _, file := range proc.EnvFiles {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, file)
		}
		if vars, err := ParseEnvFile(path); err == nil {
			for k, v := range vars {
				env[k] = v
			}
		}
	}

	// 4. Load process.env_file (relative to working_dir)
	if proc.EnvFile != "" {
		path := proc.EnvFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		if vars, err := ParseEnvFile(path); err == nil {
			for k, v := range vars {
				env[k] = v
			}
		}
	}

	// 5. Merge process.environment (highest priority)
	for k, v := range proc.Environment {
		env[k] = v
	}

	return env, nil
}
