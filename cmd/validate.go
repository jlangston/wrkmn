package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"wrkmn/internal/config"
	"wrkmn/internal/preflight"
)

// validateCmd checks config + environment without starting any process.
var validateCmd = &cobra.Command{
	Use:   "validate [directory]",
	Short: "Validate configuration and environment without starting anything",
	Long: `Loads worker-compose.yaml, validates it (types, dependencies, cycles,
dev modes), and runs pre-flight checks: cf CLI on PATH (if any worker
processes are configured), commands on PATH, working directories,
unmigrated Wrangler projects, and env files. A configured inspector_port
is flagged as a deprecated, ignored setting.

Exit code 0 when there are no errors (warnings allowed), 1 otherwise.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		os.Exit(runValidate(dir, cfgFile, os.Stdout))
	},
}

// runValidate loads the config, runs pre-flight checks, writes a report, and
// returns the process exit code.
func runValidate(dir, cfgPath string, out io.Writer) int {
	var cfg *config.Config
	var err error
	if cfgPath != "" {
		cfg, err = config.NewLoader(cfgPath).Load()
	} else {
		cfg, err = config.LoadOrDefault(dir)
	}
	if err != nil {
		fmt.Fprintf(out, "✗ config: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "✓ config valid (%d processes)\n", len(cfg.Processes))

	issues := preflight.Check(cfg)
	for _, issue := range issues {
		scope := issue.Process
		if scope == "" {
			scope = "config"
		}
		marker := "⚠"
		if issue.Severity == preflight.SeverityError {
			marker = "✗"
		}
		fmt.Fprintf(out, "%s %s: %s\n", marker, scope, issue.Message)
	}

	if preflight.HasErrors(issues) {
		return 1
	}
	if len(issues) == 0 {
		fmt.Fprintln(out, "✓ pre-flight checks passed")
	}
	return 0
}
