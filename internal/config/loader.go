package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"wrkmn/internal/theme"
)

// ConfigFileNames lists the config file names to search for in order
var ConfigFileNames = []string{
	"worker-compose.yaml",
	"worker-compose.yml",
	".worker-compose.yaml",
	".worker-compose.yml",
}

// Loader handles loading and parsing configuration files
type Loader struct {
	configPath string
	basePath   string // directory containing config file
}

// NewLoader creates a new config loader
func NewLoader(configPath string) *Loader {
	return &Loader{
		configPath: configPath,
		basePath:   filepath.Dir(configPath),
	}
}

// FindConfigFile searches for a config file in the given directory
func FindConfigFile(dir string) string {
	for _, name := range ConfigFileNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// Load reads and parses the configuration file
func (l *Loader) Load() (*Config, error) {
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config for backwards compatibility
			return l.defaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := l.validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	l.applyDefaults(&cfg)
	l.resolveWorkingDirs(&cfg)

	cfg.BaseDir = l.basePath
	return &cfg, nil
}

// LoadOrDefault loads the config file or returns defaults if not found
func LoadOrDefault(dir string) (*Config, error) {
	configPath := FindConfigFile(dir)
	if configPath == "" {
		// No config file found, return defaults
		return defaultConfigForDir(dir), nil
	}

	loader := NewLoader(configPath)
	return loader.Load()
}

// defaultConfig returns the default configuration
func (l *Loader) defaultConfig() *Config {
	return defaultConfigForDir(l.basePath)
}

func defaultConfigForDir(dir string) *Config {
	return &Config{
		Version: "1.0",
		Discovery: DiscoveryConfig{
			Enabled:     true,
			Paths:       []string{dir},
			ScriptNames: []string{"dev", "start"},
		},
		Processes: make(map[string]Process),
		BaseDir:   dir,
	}
}

// validate checks the configuration for errors
func (l *Loader) validate(cfg *Config) error {
	// Validate no circular dependencies
	if err := l.validateDependencyGraph(cfg); err != nil {
		return err
	}

	// Validate health check and startup delay are mutually exclusive
	for name, proc := range cfg.Processes {
		if proc.HealthCheck != nil && proc.StartupDelay.Duration > 0 {
			return fmt.Errorf("process %q: cannot specify both health_check and startup_delay", name)
		}
	}

	// Validate health check types
	for name, proc := range cfg.Processes {
		if proc.HealthCheck != nil {
			switch proc.HealthCheck.Type {
			case HealthCheckHTTP, HealthCheckTCP, HealthCheckExec:
				// Valid
			case "":
				return fmt.Errorf("process %q: health_check.type is required", name)
			default:
				return fmt.Errorf("process %q: invalid health_check.type %q (must be http, tcp, or exec)", name, proc.HealthCheck.Type)
			}
		}
	}

	// Validate dependency conditions
	for name, proc := range cfg.Processes {
		for _, dep := range proc.DependsOn {
			switch dep.Condition {
			case ConditionStarted, ConditionHealthy, ConditionCompleted:
				// Valid
			case "":
				return fmt.Errorf("process %q: depends_on[%s].condition is required", name, dep.Process)
			default:
				return fmt.Errorf("process %q: invalid depends_on[%s].condition %q (must be process_started, process_healthy, or process_completed)", name, dep.Process, dep.Condition)
			}

			// Check that dependency exists
			target, exists := cfg.Processes[dep.Process]
			if !exists {
				return fmt.Errorf("process %q: depends on unknown process %q", name, dep.Process)
			}
			if dep.Condition == ConditionCompleted && target.Type != ProcessTypeOneshot {
				return fmt.Errorf("process %q: process_completed requires %q to be type oneshot", name, dep.Process)
			}
		}
	}

	// Validate process types
	for name, proc := range cfg.Processes {
		switch proc.Type {
		case "", ProcessTypeWorker, ProcessTypeService, ProcessTypeOneshot:
			// Valid (empty defaults later)
		default:
			return fmt.Errorf("process %q: invalid type %q (must be worker, service, or oneshot)", name, proc.Type)
		}
		if proc.Type == ProcessTypeOneshot && proc.HealthCheck != nil {
			return fmt.Errorf("process %q: oneshot processes cannot have a health_check (they exit when done)", name)
		}
	}

	// Validate dev_mode
	for name, proc := range cfg.Processes {
		switch proc.DevMode {
		case "", DevModeLocal, DevModeRemoteBindings:
			// Valid (empty defaults later)
		default:
			return fmt.Errorf("process %q: invalid dev_mode %q (must be local or remote-bindings; full-remote dev does not exist in the cf CLI)", name, proc.DevMode)
		}
	}

	// Validate modes
	for name, proc := range cfg.Processes {
		seen := make(map[string]bool)
		for _, m := range proc.Modes {
			if m == "" {
				return fmt.Errorf("process %q: modes entries must be non-empty strings", name)
			}
			if seen[m] {
				return fmt.Errorf("process %q: duplicate mode %q", name, m)
			}
			seen[m] = true
		}
	}

	return nil
}

// validateDependencyGraph checks for circular dependencies using DFS
func (l *Loader) validateDependencyGraph(cfg *Config) error {
	// Build adjacency list
	deps := make(map[string][]string)
	for name, proc := range cfg.Processes {
		for _, dep := range proc.DependsOn {
			deps[name] = append(deps[name], dep.Process)
		}
	}

	// Track visited state: 0 = unvisited, 1 = visiting, 2 = visited
	visited := make(map[string]int)
	var path []string

	var dfs func(node string) error
	dfs = func(node string) error {
		if visited[node] == 1 {
			// Found cycle, build path string
			cycleStart := 0
			for i, n := range path {
				if n == node {
					cycleStart = i
					break
				}
			}
			cycle := append(path[cycleStart:], node)
			return fmt.Errorf("circular dependency detected: %v", cycle)
		}
		if visited[node] == 2 {
			return nil
		}

		visited[node] = 1
		path = append(path, node)

		for _, dep := range deps[node] {
			if err := dfs(dep); err != nil {
				return err
			}
		}

		path = path[:len(path)-1]
		visited[node] = 2
		return nil
	}

	for name := range cfg.Processes {
		if visited[name] == 0 {
			if err := dfs(name); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyDefaults applies default values to the configuration
func (l *Loader) applyDefaults(cfg *Config) {
	// Apply discovery defaults
	if cfg.Discovery.Paths == nil {
		cfg.Discovery.Paths = []string{"."}
	}
	if cfg.Discovery.ScriptNames == nil {
		cfg.Discovery.ScriptNames = []string{"dev", "start"}
	}

	// Apply process defaults
	for name, proc := range cfg.Processes {
		// Apply global defaults
		if cfg.Defaults.Environment != nil && proc.Environment == nil {
			proc.Environment = make(map[string]string)
		}
		for k, v := range cfg.Defaults.Environment {
			if _, exists := proc.Environment[k]; !exists {
				proc.Environment[k] = v
			}
		}

		// Apply health check defaults
		if proc.HealthCheck != nil {
			if proc.HealthCheck.Interval.Duration == 0 {
				proc.HealthCheck.Interval.Duration = DefaultHealthCheckInterval
			}
			if proc.HealthCheck.Timeout.Duration == 0 {
				proc.HealthCheck.Timeout.Duration = DefaultHealthCheckTimeout
			}
			if proc.HealthCheck.Retries == 0 {
				proc.HealthCheck.Retries = DefaultHealthCheckRetries
			}
		}

		// Inherit restart policy from defaults when the process doesn't set one
		if proc.Restart == nil && cfg.Defaults.Restart != nil {
			policy := *cfg.Defaults.Restart
			proc.Restart = &policy
		}

		// Apply restart policy defaults
		if proc.Restart != nil {
			if proc.Restart.MaxTries == 0 {
				proc.Restart.MaxTries = 3
			}
			if proc.Restart.Delay.Duration == 0 {
				proc.Restart.Delay.Duration = time.Second
			}
		}

		// Set default process type
		if proc.Type == "" {
			proc.Type = ProcessTypeService
		}

		// Default workers to remote-bindings (today's plain `wrangler dev` behavior)
		if proc.Type == ProcessTypeWorker && proc.DevMode == "" {
			proc.DevMode = DevModeRemoteBindings
		}

		// Default modes for workers (cf dev --mode values the selector offers)
		if proc.Type == ProcessTypeWorker && len(proc.Modes) == 0 {
			proc.Modes = []string{"development", "production"}
		}

		// Mark as config-sourced
		proc.Source = SourceConfig

		cfg.Processes[name] = proc
	}
}

// resolveWorkingDirs converts relative working directories to absolute paths
func (l *Loader) resolveWorkingDirs(cfg *Config) {
	for name, proc := range cfg.Processes {
		if proc.WorkingDir != "" && !filepath.IsAbs(proc.WorkingDir) {
			proc.WorkingDir = filepath.Join(l.basePath, proc.WorkingDir)
			cfg.Processes[name] = proc
		}
	}

	// Also resolve discovery paths
	for i, path := range cfg.Discovery.Paths {
		if !filepath.IsAbs(path) {
			cfg.Discovery.Paths[i] = filepath.Join(l.basePath, path)
		}
	}
}

// ApplyUIConfig applies the UI configuration from the config file to the theme system
func ApplyUIConfig(cfg *Config) {
	if cfg == nil {
		return
	}

	// Apply theme
	if cfg.UI.Theme != "" {
		themeName := strings.ToLower(cfg.UI.Theme)
		if t, found := theme.GetThemeByName(themeName); found {
			theme.SetTheme(t)
		} else {
			// Try case-insensitive match
			for _, t := range theme.Available() {
				if strings.EqualFold(t.Name, cfg.UI.Theme) {
					theme.SetTheme(t)
					break
				}
			}
		}
	}

	// Apply transparency
	if cfg.UI.TransparentBg {
		theme.SetTransparent(true)
	}
}
