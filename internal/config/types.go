package config

import (
	"time"
)

// Config is the root configuration structure
type Config struct {
	Version   string             `yaml:"version"`
	Discovery DiscoveryConfig    `yaml:"discovery"`
	Processes map[string]Process `yaml:"processes"`
	Defaults  ProcessDefaults    `yaml:"defaults,omitempty"`
	UI        UIConfig           `yaml:"ui,omitempty"`

	// BaseDir is the directory containing the config file (not from YAML)
	BaseDir string `yaml:"-"`
}

// UIConfig configures the TUI appearance
type UIConfig struct {
	Theme             string `yaml:"theme,omitempty"`              // Theme name: default, dracula, catppuccin, nord
	TransparentBg     bool   `yaml:"transparent_background,omitempty"` // Use terminal's background
	AnimationsEnabled bool   `yaml:"animations,omitempty"`         // Enable animations (default: true)
}

// DiscoveryConfig controls auto-discovery behavior
type DiscoveryConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Paths       []string `yaml:"paths"`
	Exclude     []string `yaml:"exclude,omitempty"`
	ScriptNames []string `yaml:"script_names,omitempty"`
}

// Process defines an arbitrary process
type Process struct {
	Name          string            `yaml:"name,omitempty"`
	Command       string            `yaml:"command"`
	Args          []string          `yaml:"args,omitempty"`
	WorkingDir    string            `yaml:"working_dir,omitempty"`
	EnvFile       string            `yaml:"env_file,omitempty"`
	EnvFiles      []string          `yaml:"env_files,omitempty"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	DependsOn     []Dependency      `yaml:"depends_on,omitempty"`
	HealthCheck   *HealthCheck      `yaml:"health_check,omitempty"`
	StartupDelay  Duration          `yaml:"startup_delay,omitempty"`
	InspectorPort int               `yaml:"inspector_port,omitempty"`
	DevMode       DevMode           `yaml:"dev_mode,omitempty"`
	Restart       *RestartPolicy    `yaml:"restart,omitempty"`
	LogPrefix     string            `yaml:"log_prefix,omitempty"`
	Type          ProcessType       `yaml:"type,omitempty"`

	// Internal tracking (not from YAML)
	Source ProcessSource `yaml:"-"`
}

// ProcessType distinguishes between CF workers and other processes
type ProcessType string

const (
	ProcessTypeWorker  ProcessType = "worker"
	ProcessTypeService ProcessType = "service"
	// ProcessTypeOneshot runs once; exit 0 marks it completed, anything else failed.
	ProcessTypeOneshot ProcessType = "oneshot"
)

// DevMode selects how a worker's `wrangler dev` connects to resources.
type DevMode string

const (
	// DevModeLocal forces all bindings to local simulations (wrangler dev --local).
	DevModeLocal DevMode = "local"
	// DevModeRemoteBindings runs code locally but honors the worker's own
	// `remote: true` bindings (plain wrangler dev). Debugger works.
	DevModeRemoteBindings DevMode = "remote-bindings"
	// DevModeRemote runs code on Cloudflare's edge (wrangler dev --remote).
	// No local breakpoint debugging.
	DevModeRemote DevMode = "remote"
)

// ProcessSource indicates where a process definition came from
type ProcessSource string

const (
	SourceConfig     ProcessSource = "config"
	SourceDiscovered ProcessSource = "discovered"
)

// Dependency defines a process dependency with condition
type Dependency struct {
	Process   string              `yaml:"process"`
	Condition DependencyCondition `yaml:"condition"`
}

// DependencyCondition specifies when a dependency is satisfied
type DependencyCondition string

const (
	ConditionStarted DependencyCondition = "process_started"
	ConditionHealthy DependencyCondition = "process_healthy"
	// ConditionCompleted is satisfied when a oneshot dependency exits 0.
	ConditionCompleted DependencyCondition = "process_completed"
)

// HealthCheck defines how to verify process health
type HealthCheck struct {
	Type        HealthCheckType `yaml:"type"`
	Target      string          `yaml:"target"`
	Interval    Duration        `yaml:"interval,omitempty"`
	Timeout     Duration        `yaml:"timeout,omitempty"`
	Retries     int             `yaml:"retries,omitempty"`
	StartPeriod Duration        `yaml:"start_period,omitempty"`
}

// HealthCheckType specifies the health check method
type HealthCheckType string

const (
	HealthCheckHTTP HealthCheckType = "http"
	HealthCheckTCP  HealthCheckType = "tcp"
	HealthCheckExec HealthCheckType = "exec"
)

// RestartPolicy defines restart behavior
type RestartPolicy struct {
	Enabled  bool     `yaml:"enabled"`
	MaxTries int      `yaml:"max_tries,omitempty"`
	Delay    Duration `yaml:"delay,omitempty"`
}

// ProcessDefaults provides defaults applied to all processes
type ProcessDefaults struct {
	EnvFile     string            `yaml:"env_file,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	HealthCheck *HealthCheck      `yaml:"health_check,omitempty"`
	Restart     *RestartPolicy    `yaml:"restart,omitempty"`
}

// Duration wraps time.Duration for YAML unmarshaling
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		// Try as number (seconds)
		var secs float64
		if err := unmarshal(&secs); err != nil {
			return err
		}
		d.Duration = time.Duration(secs * float64(time.Second))
		return nil
	}
	var err error
	d.Duration, err = time.ParseDuration(s)
	return err
}

// MarshalYAML implements yaml.Marshaler for Duration
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// DefaultHealthCheckInterval is the default interval between health checks
const DefaultHealthCheckInterval = 5 * time.Second

// DefaultHealthCheckTimeout is the default timeout for health checks
const DefaultHealthCheckTimeout = 3 * time.Second

// DefaultHealthCheckRetries is the default number of retries before marking unhealthy
const DefaultHealthCheckRetries = 3
