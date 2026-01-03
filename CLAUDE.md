# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**wrkmn** - A powerful TUI for managing Cloudflare Workers and related processes. Think of it as process-compose meets k9s, specifically designed for CF Worker development.

## Build & Run Commands

```bash
# Build the binary
go build -o wrkmn .

# Build with version info
go build -ldflags "-X wrkmn/cmd.Version=1.0.0 -X wrkmn/cmd.Commit=$(git rev-parse HEAD) -X wrkmn/cmd.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o wrkmn .

# Run in current directory
./wrkmn

# Run with custom path
./wrkmn /path/to/your/project

# Run with theme
./wrkmn --theme catppuccin --transparent

# List available themes
./wrkmn themes

# Show version
./wrkmn version
```

## CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file (default: worker-compose.yaml) |
| `--theme` | `-t` | Color theme: default, dracula, catppuccin, nord |
| `--transparent` | | Use terminal's background color |
| `--no-discovery` | | Disable auto-discovery of workers |
| `--help` | `-h` | Show help |

## Configuration (worker-compose.yaml)

```yaml
version: "1.0"

ui:
  theme: catppuccin
  transparent_background: true
  animations: true

discovery:
  enabled: true
  paths: [.]
  exclude: ["**/node_modules/**"]

processes:
  postgres:
    command: docker
    args: [compose, up, postgres]
    type: service
    health_check:
      type: tcp
      target: localhost:5432
      interval: 2s
      retries: 10

  my-worker:
    command: pnpm
    args: [run, dev]
    working_dir: ./my-worker
    type: worker
    inspector_port: 9229
    depends_on:
      - process: postgres
        condition: process_healthy

defaults:
  restart:
    enabled: true
    max_tries: 3
```

## Architecture

### Package Structure

```
├── main.go                      # Entry point, calls cmd.Execute()
├── cmd/
│   └── root.go                  # Cobra CLI commands and flags
├── internal/
│   ├── config/                  # YAML configuration
│   │   ├── types.go             # Config structs (Process, HealthCheck, etc.)
│   │   └── loader.go            # YAML parsing, validation, defaults
│   │
│   ├── worker/                  # Worker discovery
│   │   ├── types.go             # Worker struct
│   │   └── discoverer.go        # Scans for wrangler.toml/package.json
│   │
│   ├── process/                 # Process lifecycle
│   │   ├── manager.go           # Start/stop processes, log capture
│   │   ├── health.go            # HTTP/TCP/exec health checks
│   │   ├── dependency.go        # DAG, topological sort (Kahn's algorithm)
│   │   └── scheduler.go         # Orchestrated startup with dependencies
│   │
│   ├── debug/                   # CDP debugging (CF Workers only)
│   │   ├── types.go             # Session, Breakpoint, CallFrame, etc.
│   │   ├── inspector.go         # V8 inspector discovery (port 9229+)
│   │   ├── session.go           # CDP WebSocket connection
│   │   ├── breakpoints.go       # Set/remove/list breakpoints
│   │   ├── stepping.go          # Step over/into/out, resume
│   │   ├── evaluation.go        # Runtime.evaluate() for REPL
│   │   └── source.go            # Source file mapping and caching
│   │
│   ├── vi/                      # VI mode (bash vi-mode parity)
│   │   ├── types.go             # Mode, Key, Cursor, Register
│   │   ├── model.go             # Bubble Tea integration
│   │   ├── buffer.go            # Text buffer with undo stack
│   │   ├── motions.go           # h/l, w/b/e, 0/$, %, f/t
│   │   ├── operators.go         # d, c, y with motions
│   │   ├── find.go              # f/F/t/T character finding
│   │   ├── search.go            # /pattern, ?pattern, n/N
│   │   └── history.go           # Command history ring
│   │
│   ├── theme/                   # Color themes
│   │   ├── theme.go             # Theme struct, transparency support
│   │   ├── default.go           # Default (cool blues)
│   │   ├── dracula.go           # Dracula (purple/pink)
│   │   ├── catppuccin.go        # Catppuccin Mocha (pastels)
│   │   └── nord.go              # Nord (arctic blue)
│   │
│   └── ui/                      # TUI components
│       ├── model.go             # Main Bubble Tea model
│       ├── views.go             # ViewMode enum, view switching
│       ├── styles.go            # Theme-aware style helpers
│       ├── table.go             # Process table view
│       ├── logs.go              # Log viewer with viewport
│       ├── help.go              # Help overlay
│       ├── toast.go             # Notification toasts
│       ├── sparkline.go         # CPU/memory charts
│       ├── source_view.go       # Source code with breakpoint gutter
│       ├── callstack_view.go    # Call stack frames
│       ├── variables_view.go    # Variable tree inspection
│       ├── console_view.go      # REPL with VI mode
│       └── debug_view.go        # Debug layout coordinator
```

### Key Concepts

**Process Types:**
- `worker` - CF Worker with inspector port for debugging
- `service` - Generic process (docker, database, etc.)

**Dependency Conditions:**
- `process_started` - Wait for process to start
- `process_healthy` - Wait for health check to pass

**Health Check Types:**
- `http` - HTTP GET returns 2xx
- `tcp` - TCP connection succeeds
- `exec` - Command exits with 0

**Debug Features (CF Workers only):**
- Breakpoints (F9 or 'b')
- Step over (F10), step into (F11), step out (Shift+F11)
- Continue (F5)
- Variable inspection with tree expansion
- REPL console with VI mode

### Data Flow

1. `cmd/root.go` parses flags and loads config
2. `config.ApplyUIConfig()` sets theme and transparency
3. `Discoverer` scans for workers if discovery enabled
4. `Scheduler` builds dependency graph and starts processes in order
5. `HealthChecker` monitors process health
6. TUI displays status, logs, and debug views
7. Debug session connects via CDP WebSocket to inspector ports

### Key Keybindings

**Main View:**
- `j/k` - Navigate up/down
- `Enter` - Start/stop selected process
- `l` - View logs
- `d` - Enter debug mode (workers only)
- `a` - Start all
- `x` - Stop all
- `q` - Quit

**Debug View:**
- `Tab` - Switch panels (source/callstack/variables/console)
- `F5` - Continue
- `F9` - Toggle breakpoint
- `F10` - Step over
- `F11` - Step into
- `Shift+F11` - Step out

**VI Mode (console):**
- `i/a/A` - Enter insert mode
- `Esc` - Normal mode
- `h/l` - Left/right
- `w/b/e` - Word navigation
- `d{motion}` - Delete
- `c{motion}` - Change
- `y{motion}` - Yank
- `p/P` - Paste
- `u` - Undo

### Dependencies

```go
// TUI Framework
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles
github.com/charmbracelet/lipgloss

// CLI
github.com/spf13/cobra

// Config
gopkg.in/yaml.v3

// WebSocket (CDP)
github.com/gorilla/websocket
```

### Theme System

Themes support transparent backgrounds for terminals with background images:

```go
// Enable transparency
theme.SetTransparent(true)

// Or in config
ui:
  transparent_background: true
```

Use `theme.Current.BgColor()` instead of `theme.Current.Background` to respect transparency setting.

### Adding a New Theme

1. Create `internal/theme/mytheme.go`
2. Define `var MyTheme = Theme{...}` with all color fields
3. Add to `Available()` in `theme.go`
