// Package preflight inspects a wrkmn configuration and the local environment
// for problems that would otherwise surface as cryptic child-process failures.
// It never starts anything.
package preflight

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"time"

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

const dialTimeout = 250 * time.Millisecond

// Check inspects the configuration and environment. Deterministic order
// (processes sorted by name) so reports are stable.
func Check(cfg *config.Config) []Issue {
	var issues []Issue

	names := make([]string, 0, len(cfg.Processes))
	for name := range cfg.Processes {
		names = append(names, name)
	}
	sort.Strings(names)

	inspectorPorts := make(map[int][]string)

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

		if proc.InspectorPort > 0 {
			inspectorPorts[proc.InspectorPort] = append(inspectorPorts[proc.InspectorPort], name)
			addr := fmt.Sprintf("localhost:%d", proc.InspectorPort)
			if conn, err := net.DialTimeout("tcp", addr, dialTimeout); err == nil {
				conn.Close()
				issues = append(issues, Issue{SeverityWarning, name, fmt.Sprintf("inspector port %d is already in use", proc.InspectorPort)})
			}
		}
	}

	ports := make([]int, 0, len(inspectorPorts))
	for port := range inspectorPorts {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	for _, port := range ports {
		if procs := inspectorPorts[port]; len(procs) > 1 {
			sort.Strings(procs)
			issues = append(issues, Issue{SeverityError, "", fmt.Sprintf("inspector port %d used by multiple processes: %v", port, procs)})
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
