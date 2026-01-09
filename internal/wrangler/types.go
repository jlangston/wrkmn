package wrangler

// Config represents a wrangler.toml configuration file
type Config struct {
	Name               string                 `toml:"name"`
	AccountID          string                 `toml:"account_id"`
	CompatibilityDate  string                 `toml:"compatibility_date"`
	CompatibilityFlags []string               `toml:"compatibility_flags"`
	Main               string                 `toml:"main"`

	// Environment-specific configurations
	Env map[string]Environment `toml:"env"`

	// Other common fields
	Workers    WorkersConfig  `toml:"workers"`
	Vars       map[string]any `toml:"vars"`
	Routes     []Route        `toml:"routes"`
	Route      string         `toml:"route"`
	Triggers   Triggers       `toml:"triggers"`
	Build      BuildConfig    `toml:"build"`
}

// Environment represents an environment-specific configuration in wrangler.toml
// Environments can override most root-level settings
type Environment struct {
	Name               string         `toml:"name"`
	AccountID          string         `toml:"account_id"`
	CompatibilityDate  string         `toml:"compatibility_date"`
	CompatibilityFlags []string       `toml:"compatibility_flags"`
	Vars               map[string]any `toml:"vars"`
	Routes             []Route        `toml:"routes"`
	Route              string         `toml:"route"`
	Workers            WorkersConfig  `toml:"workers"`
	Triggers           Triggers       `toml:"triggers"`
}

// WorkersConfig represents the workers.dev settings
type WorkersConfig struct {
	Dev bool `toml:"dev"`
}

// Route represents a route configuration
type Route struct {
	Pattern      string `toml:"pattern"`
	ZoneID       string `toml:"zone_id"`
	ZoneName     string `toml:"zone_name"`
	CustomDomain bool   `toml:"custom_domain"`
}

// Triggers represents cron triggers
type Triggers struct {
	Crons []string `toml:"crons"`
}

// BuildConfig represents build configuration
type BuildConfig struct {
	Command   string `toml:"command"`
	CwdPrefix string `toml:"cwd"`
	Watch     string `toml:"watch_dir"`
}

// ResolvedEnvironment contains the fully resolved configuration for an environment
// with all inherited values from root config
type ResolvedEnvironment struct {
	Name               string
	AccountID          string
	CompatibilityDate  string
	CompatibilityFlags []string
	Vars               map[string]any
	Routes             []Route
}

// GetAccountID returns the account ID for a given environment.
// Returns the environment's account_id if set, otherwise falls back to root account_id.
func (c *Config) GetAccountID(envName string) string {
	if envName == "" || envName == "production" {
		return c.AccountID
	}

	if env, ok := c.Env[envName]; ok && env.AccountID != "" {
		return env.AccountID
	}

	return c.AccountID
}

// GetEnvironments returns a list of available environment names
func (c *Config) GetEnvironments() []string {
	envs := make([]string, 0, len(c.Env)+1)

	// "production" is the implicit root environment
	envs = append(envs, "production")

	for name := range c.Env {
		envs = append(envs, name)
	}

	return envs
}

// ResolveEnvironment returns a fully resolved environment configuration
// with values inherited from the root config
func (c *Config) ResolveEnvironment(envName string) ResolvedEnvironment {
	resolved := ResolvedEnvironment{
		Name:               envName,
		AccountID:          c.AccountID,
		CompatibilityDate:  c.CompatibilityDate,
		CompatibilityFlags: c.CompatibilityFlags,
		Vars:               make(map[string]any),
		Routes:             c.Routes,
	}

	// Copy root vars
	for k, v := range c.Vars {
		resolved.Vars[k] = v
	}

	// If specific environment exists, override with its values
	if envName != "" && envName != "production" {
		if env, ok := c.Env[envName]; ok {
			if env.AccountID != "" {
				resolved.AccountID = env.AccountID
			}
			if env.CompatibilityDate != "" {
				resolved.CompatibilityDate = env.CompatibilityDate
			}
			if len(env.CompatibilityFlags) > 0 {
				resolved.CompatibilityFlags = env.CompatibilityFlags
			}
			if len(env.Routes) > 0 {
				resolved.Routes = env.Routes
			}
			// Merge vars (env vars override root vars)
			for k, v := range env.Vars {
				resolved.Vars[k] = v
			}
		}
	}

	return resolved
}
