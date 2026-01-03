package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDuration_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"seconds string", "interval: 5s", 5 * time.Second, false},
		{"minutes string", "interval: 2m", 2 * time.Minute, false},
		{"milliseconds string", "interval: 500ms", 500 * time.Millisecond, false},
		{"complex duration", "interval: 1h30m", 90 * time.Minute, false},
		{"invalid format", "interval: invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				Interval Duration `yaml:"interval"`
			}

			err := yaml.Unmarshal([]byte(tt.input), &result)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result.Interval.Duration)
			}
		})
	}
}

func TestDuration_MarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		expected string
	}{
		{"seconds", Duration{5 * time.Second}, "5s"},
		{"minutes", Duration{2 * time.Minute}, "2m0s"},
		{"complex", Duration{90 * time.Minute}, "1h30m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.duration.MarshalYAML()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessType_Constants(t *testing.T) {
	assert.Equal(t, ProcessType("worker"), ProcessTypeWorker)
	assert.Equal(t, ProcessType("service"), ProcessTypeService)
}

func TestProcessSource_Constants(t *testing.T) {
	assert.Equal(t, ProcessSource("config"), SourceConfig)
	assert.Equal(t, ProcessSource("discovered"), SourceDiscovered)
}

func TestHealthCheckType_Constants(t *testing.T) {
	assert.Equal(t, HealthCheckType("http"), HealthCheckHTTP)
	assert.Equal(t, HealthCheckType("tcp"), HealthCheckTCP)
	assert.Equal(t, HealthCheckType("exec"), HealthCheckExec)
}

func TestDependencyCondition_Constants(t *testing.T) {
	assert.Equal(t, DependencyCondition("process_started"), ConditionStarted)
	assert.Equal(t, DependencyCondition("process_healthy"), ConditionHealthy)
}

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, 5*time.Second, DefaultHealthCheckInterval)
	assert.Equal(t, 3*time.Second, DefaultHealthCheckTimeout)
	assert.Equal(t, 3, DefaultHealthCheckRetries)
}

func TestConfig_YAML_Parsing(t *testing.T) {
	yamlData := `
version: "1.0"
ui:
  theme: dracula
  transparent_background: true
  animations: false
discovery:
  enabled: true
  paths:
    - ./workers
    - ./services
  exclude:
    - "**/node_modules/**"
processes:
  api:
    command: npm
    args: [run, dev]
    working_dir: ./api
    type: service
    health_check:
      type: http
      target: http://localhost:3000/health
      interval: 5s
      timeout: 3s
      retries: 3
    depends_on:
      - process: db
        condition: process_healthy
  db:
    command: docker
    args: [compose, up, postgres]
    type: service
defaults:
  restart:
    enabled: true
    max_tries: 5
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	require.NoError(t, err)

	// Verify parsed values
	assert.Equal(t, "1.0", cfg.Version)
	assert.Equal(t, "dracula", cfg.UI.Theme)
	assert.True(t, cfg.UI.TransparentBg)
	assert.False(t, cfg.UI.AnimationsEnabled)
	assert.True(t, cfg.Discovery.Enabled)
	assert.Len(t, cfg.Discovery.Paths, 2)
	assert.Len(t, cfg.Processes, 2)

	// Verify api process
	api := cfg.Processes["api"]
	assert.Equal(t, "npm", api.Command)
	assert.Equal(t, []string{"run", "dev"}, api.Args)
	assert.Equal(t, "./api", api.WorkingDir)
	assert.Equal(t, ProcessTypeService, api.Type)
	require.NotNil(t, api.HealthCheck)
	assert.Equal(t, HealthCheckHTTP, api.HealthCheck.Type)
	assert.Equal(t, "http://localhost:3000/health", api.HealthCheck.Target)
	assert.Equal(t, 5*time.Second, api.HealthCheck.Interval.Duration)
	assert.Len(t, api.DependsOn, 1)
	assert.Equal(t, "db", api.DependsOn[0].Process)
	assert.Equal(t, ConditionHealthy, api.DependsOn[0].Condition)

	// Verify defaults
	require.NotNil(t, cfg.Defaults.Restart)
	assert.True(t, cfg.Defaults.Restart.Enabled)
	assert.Equal(t, 5, cfg.Defaults.Restart.MaxTries)
}

func TestProcess_WithEnvironment(t *testing.T) {
	yamlData := `
processes:
  test:
    command: node
    args: [app.js]
    environment:
      NODE_ENV: production
      PORT: "3000"
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	require.NoError(t, err)

	proc := cfg.Processes["test"]
	assert.Equal(t, "production", proc.Environment["NODE_ENV"])
	assert.Equal(t, "3000", proc.Environment["PORT"])
}

func TestProcess_WithInspectorPort(t *testing.T) {
	yamlData := `
processes:
  worker:
    command: wrangler
    args: [dev]
    inspector_port: 9229
    type: worker
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	require.NoError(t, err)

	proc := cfg.Processes["worker"]
	assert.Equal(t, 9229, proc.InspectorPort)
	assert.Equal(t, ProcessTypeWorker, proc.Type)
}
