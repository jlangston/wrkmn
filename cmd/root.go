package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"wrkmn/internal/config"
	"wrkmn/internal/theme"
	"wrkmn/internal/ui"
	"wrkmn/internal/worker"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	// Version info (set via ldflags)
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"

	// CLI flags
	cfgFile     string
	themeName   string
	transparent bool
	noDiscovery bool
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "wrkmn [directory]",
	Short: "A TUI for managing Cloudflare Workers",
	Long: `wrkmn - A powerful TUI for managing Cloudflare Workers and related processes.

Features:
  - Auto-discovery of CF Workers in your project
  - Process orchestration with dependencies and health checks
  - Full debugging support (breakpoints, stepping, REPL)
  - VI mode keybindings
  - Multiple color themes with transparency support

Examples:
  wrkmn                    # Run in current directory
  wrkmn ./my-project       # Run in specified directory
  wrkmn -t catppuccin      # Use Catppuccin theme
  wrkmn --transparent      # Use terminal's background`,
	Args: cobra.MaximumNArgs(1),
	Run:  runApp,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Persistent flags (available to all subcommands)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: worker-compose.yaml)")
	rootCmd.PersistentFlags().StringVarP(&themeName, "theme", "t", "", "color theme (default, dracula, catppuccin, nord)")
	rootCmd.PersistentFlags().BoolVar(&transparent, "transparent", false, "use transparent background")

	// Local flags
	rootCmd.Flags().BoolVar(&noDiscovery, "no-discovery", false, "disable auto-discovery of workers")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(themesCmd)
}

func runApp(cmd *cobra.Command, args []string) {
	// Determine the base directory
	baseDir := "."
	if len(args) > 0 {
		baseDir = args[0]
	}

	// Load configuration
	var cfg *config.Config
	var err error

	if cfgFile != "" {
		loader := config.NewLoader(cfgFile)
		cfg, err = loader.Load()
	} else {
		cfg, err = config.LoadOrDefault(baseDir)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply CLI flag overrides
	if themeName != "" {
		cfg.UI.Theme = themeName
	}
	if transparent {
		cfg.UI.TransparentBg = true
	}
	if noDiscovery {
		cfg.Discovery.Enabled = false
	}

	// Apply UI configuration (theme, transparency)
	config.ApplyUIConfig(cfg)

	// Build workers from config processes first (they take priority)
	var workers []*worker.Worker
	configWorkerNames := make(map[string]bool)

	for name, proc := range cfg.Processes {
		w := &worker.Worker{
			Name:          name,
			Path:          proc.WorkingDir,
			Status:        worker.StatusStopped,
			InspectorPort: proc.InspectorPort,
			Scripts: map[string]string{
				"_config": proc.Command, // Special marker for config-based processes
			},
		}
		// Populate the real serving Port (and env/cloudflare metadata) from the
		// process's wrangler.toml [dev] block. Previously Port was set to the
		// inspector port, so the UI showed 9229+ instead of the port wrangler
		// actually serves on (e.g. 8787). No-op for non-worker services.
		worker.EnrichFromWranglerConfig(w)
		workers = append(workers, w)
		configWorkerNames[name] = true
	}

	// Discover additional workers (skip if already defined in config)
	if cfg.Discovery.Enabled {
		discoverer := worker.NewDiscoverer()
		for _, path := range cfg.Discovery.Paths {
			discovered, err := discoverer.DiscoverWorkers(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: discovery failed for %s: %v\n", path, err)
				continue
			}
			for _, w := range discovered {
				// Skip if already defined in config
				if configWorkerNames[w.Name] {
					continue
				}
				workers = append(workers, w)
			}
		}
	}

	if len(workers) == 0 {
		fmt.Fprintln(os.Stderr, "No workers found and no processes defined")
		fmt.Fprintln(os.Stderr, "Run 'wrkmn --help' for usage information")
		os.Exit(1)
	}

	// Create and run TUI
	m := ui.NewModelWithConfig(workers, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		// Stop all processes first for proper cleanup
		m.GetManager().StopAll()
		// Send quit message to tea program instead of os.Exit
		// This allows deferred cleanup functions to run
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	// Ensure all processes are stopped after tea program exits
	m.GetManager().StopAll()
}

// versionCmd shows version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("wrkmn %s\n", Version)
		fmt.Printf("  commit: %s\n", Commit)
		fmt.Printf("  built:  %s\n", BuildDate)
	},
}

// themesCmd lists available themes
var themesCmd = &cobra.Command{
	Use:   "themes",
	Short: "List available color themes",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available themes:")
		fmt.Println()
		for _, t := range theme.Available() {
			marker := "  "
			if t.Name == theme.Current.Name {
				marker = "* "
			}
			fmt.Printf("%s%s\n", marker, t.Name)
		}
		fmt.Println()
		fmt.Println("Use with: wrkmn --theme <name>")
		fmt.Println("Or set in worker-compose.yaml:")
		fmt.Println("  ui:")
		fmt.Println("    theme: catppuccin")
	},
}
