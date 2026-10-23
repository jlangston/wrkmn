// Package preflight inspects a wrkmn configuration and the local environment
// for problems that would otherwise surface as cryptic child-process failures.
// It never starts anything.
package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"sort"

	"wrkmn/internal/config"
)

// Severity ranks an issue: warnings are survivable, errors are not.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// Issue is a single pre-flight finding. Process is "" for config-level issues.
type Issue struct {
	Severity Severity
	Process  string
	Message  string
}

// Check inspects the configuration and environment. Deterministic order
// (processes sorted by name) so reports are stable.
func Check(cfg *config.Config) []Issue {
	var issues []Issue

	names := make([]string, 0, len(cfg.Processes))
	for name := range cfg.Processes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		proc := cfg.Processes[name]

		if _, err := exec.LookPath(proc.Command); err != nil {
			issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("command %q not found on PATH", proc.Command)})
		}

		if proc.WorkingDir != "" {
			if info, err := os.Stat(proc.WorkingDir); err != nil || !info.IsDir() {
				issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("working_dir %q is not a directory", proc.WorkingDir)})
			}
		}

		for _, ref := range config.ResolveEnvFilePaths(&proc, cfg.Defaults, cfg.BaseDir) {
			if _, err := os.Stat(ref.Path); err != nil {
				if ref.FromDefault {
					issues = append(issues, Issue{SeverityWarning, name, fmt.Sprintf("default env file %q not found (skipped at start)", ref.Path)})
				} else {
					issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("env file %q not found", ref.Path)})
				}
			}
		}
	}

	return issues
}

// HasErrors reports whether any issue is error severity.
func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}
