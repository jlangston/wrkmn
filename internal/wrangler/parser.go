package wrangler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ConfigFileName is the standard wrangler config filename
const ConfigFileName = "wrangler.toml"

// AlternativeConfigFileNames are other valid wrangler config filenames
var AlternativeConfigFileNames = []string{
	"wrangler.toml",
	"wrangler.json", // JSON format (not parsed by this package)
}

// ParseFile parses a wrangler.toml file from the given path
func ParseFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading wrangler config: %w", err)
	}

	return Parse(data)
}

// Parse parses wrangler.toml content from bytes
func Parse(data []byte) (*Config, error) {
	var config Config

	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, fmt.Errorf("parsing wrangler config: %w", err)
	}

	return &config, nil
}

// FindConfig looks for a wrangler.toml file in the given directory
func FindConfig(dir string) (string, error) {
	configPath := filepath.Join(dir, ConfigFileName)

	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	return "", fmt.Errorf("wrangler.toml not found in %s", dir)
}

// LoadFromDirectory loads the wrangler config from a directory
func LoadFromDirectory(dir string) (*Config, error) {
	configPath, err := FindConfig(dir)
	if err != nil {
		return nil, err
	}

	return ParseFile(configPath)
}

// Exists checks if a wrangler.toml exists in the given directory
func Exists(dir string) bool {
	configPath := filepath.Join(dir, ConfigFileName)
	_, err := os.Stat(configPath)
	return err == nil
}

// GetWorkerName returns the worker name from config, with fallback to directory name
func GetWorkerName(config *Config, dir string) string {
	if config != nil && config.Name != "" {
		return config.Name
	}

	// Fallback to directory name
	return filepath.Base(dir)
}
