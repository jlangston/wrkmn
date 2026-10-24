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
| `--skip-preflight` | | Skip startup pre-flight environment checks |
| `--help` | `-h` | Show help |

## Testing

```bash
# Unit tests
go test ./...

# Live cf smoke test — requires cf and bun on PATH, plus network access to
# scaffold a throwaway project. Launches a worker through the process
# manager, attaches the debugger through the same discovery path the TUI
# uses, and drives a breakpoint pause/inspect/resume. This is the documented
# way to verify a machine's cf setup end-to-end.
go test -tags cfsmoke ./test/integration/ -run TestCfLiveSmoke -v -timeout 300s
```

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
    command: cf
    args: [dev]
    working_dir: ./my-worker
    type: worker
    modes: [development, production]
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
│   ├── root.go                  # Cobra CLI commands and flags
│   └── validate.go              # `validate` subcommand (config + pre-flight check, no start)
├── internal/
│   ├── config/                  # YAML configuration
│   │   ├── types.go             # Config structs (Process, HealthCheck, etc.)
│   │   └── loader.go            # YAML parsing, validation, defaults
│   │
│   ├── preflight/               # Environment pre-flight checks
│   │   └── preflight.go         # cf on PATH, commands/working dirs, unmigrated projects, env files; inspector_port deprecation warning
│   │
│   ├── cfproject/               # cf project detection, banner parsing, debug-ws extraction
│   │   └── cfproject.go
│   │
│   ├── worker/                  # Worker discovery
│   │   ├── types.go             # Worker struct
│   │   └── discoverer.go        # Scans for cloudflare.config.{ts,mts,js,mjs}; unmigrated Wrangler dirs reported separately
│   │
│   ├── process/                 # Process lifecycle
│   │   ├── manager.go           # Start/stop processes, log capture
│   │   ├── health.go            # HTTP/TCP/exec health checks
│   │   ├── dependency.go        # DAG, topological sort (Kahn's algorithm)
│   │   └── scheduler.go         # Orchestrated startup with dependencies
│   │
│   ├── debug/                   # CDP debugging (CF Workers only)
│   │   ├── types.go             # Session, Breakpoint, CallFrame, etc.
│   │   ├── discover.go          # Finds a worker's CDP websocket: banner-discovered /__debug page, falling back to a /json port scan
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
│   ├── state/                   # Persisted runtime selections
│   │   └── state.go             # Save/load/apply dev-mode + mode selections (.wrkmn-state.json)
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
- `worker` - Cloudflare Worker launched via `cf dev`; the debugger attaches automatically
- `service` - Generic process (docker, database, etc.)
- `oneshot` - runs once; exit 0 marks it completed, anything else failed; never restarted

**Dev Modes (workers):**
- `local` → `cf dev --local` (all bindings simulated locally; debugger on)
- `remote-bindings` (default) → plain `cf dev` (honors the worker's own `remote: true` bindings; debugger on)

Full-remote dev doesn't exist in the cf CLI (it did under wrangler; removed in
the cutover). Set the dev mode per-process via `dev_mode:` in config, or
switch at runtime with `m`.

**Modes (workers):**
- `modes:` in config lists the named configs a worker's `cloudflare.config.ts`
  exports (default `[development, production]` for `type: worker`
  processes); `cf dev --mode <name>` selects between them.
- `e` selects a mode at runtime. Selecting nothing leaves cf's own default
  (`development`) in effect — wrkmn passes no `--mode` flag until one is
  chosen explicitly.
- Persisted per worker as `selected_mode` in `.wrkmn-state.json`; legacy
  files that stored `selected_env` migrate automatically on load.

**Dependency Conditions:**
- `process_started` - Wait for process to start
- `process_healthy` - Wait for health check to pass
- `process_completed` - Wait for a `oneshot` to exit 0

**Health Check Types:**
- `http` - HTTP GET returns 2xx
- `tcp` - TCP connection succeeds
- `exec` - Command exits with 0

Health checks gate startup ordering only — the scheduler tears down its
health monitors once start-all orchestration completes. wrkmn does not run
continuous background health monitoring after startup.

**Restart Policy:**
- `defaults.restart` (`enabled`, `max_tries`, `delay`) is inherited by
  `worker`/`service` processes that don't set their own `restart:`.
- A crash restarts the process, up to `max_tries` consecutive crashes; a run
  lasting 10s+ resets the counter.
- Manual stops never trigger a restart. `oneshot` processes are never
  restarted.

**Debug Features (CF Workers only):**
- Breakpoints (F9 or 'b')
- Step over (F10), step into (F11), step out (Shift+F11)
- Continue (F5)
- Variable inspection with tree expansion
- REPL console with VI mode

Attachment is automatic, no configuration needed: `D` reads the `Debug:` URL
off the `cf dev` startup banner (`internal/cfproject.ParseBannerLine`),
fetches its `/__debug` page, and extracts the CDP websocket
(`internal/cfproject.ExtractDebugWS`); if the banner hasn't appeared yet,
`internal/debug/discover.go` falls back to scanning ports 9229–9249 for an
inspector target matching the worker's name. Two implementation details worth
knowing:
- The websocket client sends `Origin: http://localhost` on connect because
  miniflare's inspector proxy rejects the handshake without an allowed
  Origin whenever a User-Agent header is present — and Go's `http.Client`
  sets one by default.
- **Known v1 limitation:** `cf dev`'s Vite SSR transform renames module-level
  bindings (`env` → `__vite_ssr_import_0__.env`), so evaluating one by its
  original name in the REPL throws a `ReferenceError`. Frame-local variables
  are unaffected.

### Data Flow

1. `cmd/root.go` parses flags and loads config
2. `config.ApplyUIConfig()` sets theme and transparency
3. `preflight.Check()` runs environment checks (cf on PATH, unmigrated Wrangler projects, commands/working dirs, env files); errors block startup, warnings pause for Enter (`--skip-preflight` bypasses this)
4. `Discoverer` scans immediate subdirectories for cf projects (`cloudflare.config.*`) if discovery enabled; directories with only a Wrangler config are reported as unmigrated instead of becoming manageable workers
5. `state.Load()`/`state.Apply()` restore prior dev-mode/mode selections from `.wrkmn-state.json` onto the built workers
6. TUI starts; pressing `a` (start all) routes config-defined processes through the `Scheduler` (dependency order, health gating, oneshot completion) while discovered workers start directly via `cf dev`
7. `HealthChecker` gates that start-all ordering — monitors are torn down once orchestration completes, so there's no continuous background health monitoring afterward
8. As a worker's `cf dev` process logs, the manager feeds each line through `cfproject.ParseBannerLine` to populate `DevServerURL`/`DebugURL` on the `Worker` once its startup banner prints
9. TUI displays status, logs, and debug views
10. Pressing `D` runs `debug.DiscoverWorkerWS`: poll the banner-discovered `DebugURL` until its `/__debug` page yields a CDP websocket, falling back to a `/json` scan of ports 9229–9249; the debug session then connects over that websocket

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
