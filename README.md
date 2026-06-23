# wrkmn

A powerful TUI for managing Cloudflare Workers and related processes. Think of it as process-compose meets k9s, specifically designed for CF Worker development.

## Features

- **Auto-discovery** - Automatically discovers Cloudflare workers by scanning for `wrangler.toml` and `package.json`
- **Process orchestration** - Manage workers alongside services like Docker, databases, etc.
- **Dependency management** - Define startup order with health check conditions
- **Health checks** - HTTP, TCP, and exec-based health monitoring
- **Live logs** - Real-time log streaming with viewport scrolling
- **Integrated debugger** - CDP-based debugging for CF Workers (breakpoints, stepping, REPL)
- **VI mode** - Bash-style vi editing in the debug console
- **Themes** - Built-in themes (default, dracula, catppuccin, nord) with transparency support

## Installation

```bash
go build -o wrkmn .
```

Or with version info:

```bash
go build -ldflags "-X wrkmn/cmd.Version=1.0.0 -X wrkmn/cmd.Commit=$(git rev-parse HEAD)" -o wrkmn .
```

## Usage

```bash
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

### CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file (default: worker-compose.yaml) |
| `--theme` | `-t` | Color theme: default, dracula, catppuccin, nord |
| `--transparent` | | Use terminal's background color |
| `--no-discovery` | | Disable auto-discovery of workers |
| `--help` | `-h` | Show help |

## Configuration

Create a `worker-compose.yaml` in your project root:

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

### Process Types

- `worker` - CF Worker with inspector port for debugging
- `service` - Generic process (docker, database, etc.)

## Dev Modes

Each worker runs in one of three modes (press `m` to switch; set a default with `dev_mode:` in `worker-compose.yaml`):

| Mode | `wrangler dev` | Bindings | Debugger |
| --- | --- | --- | --- |
| `local` | `--local` | all local simulations | ✅ |
| `remote-bindings` (default) | _(none)_ | honors each worker's `remote: true` bindings | ✅ |
| `remote` (full-remote) | `--remote` | all remote, code on edge | ❌ |

**Full-remote caveats:** slower iteration (upload per change), a 50-routes-per-zone limit, deployed secrets are not loaded (use local `.dev.vars`), and breakpoint debugging is unavailable. wrkmn asks for confirmation before starting a worker in this mode.

### Dependency Conditions

- `process_started` - Wait for process to start
- `process_healthy` - Wait for health check to pass

### Health Check Types

- `http` - HTTP GET returns 2xx
- `tcp` - TCP connection succeeds
- `exec` - Command exits with 0

## Keybindings

### Main View

| Key | Action |
|-----|--------|
| `j/k` | Navigate up/down |
| `Enter` / `l` | View logs |
| `d` | Start (dev) |
| `D` | Enter debug mode (running CF Workers only) |
| `m` | Select dev mode (local / remote-bindings / full-remote) |
| `s` | Run a script |
| `r` | Remote logs (CF Workers) |
| `e` | Select environment |
| `x` | Stop process |
| `a` | Start all |
| `z` | Stop all |
| `?` | Help |
| `q` | Quit |

### Log View

| Key | Action |
|-----|--------|
| `j/k` | Scroll down/up |
| `g/G` | Top/bottom |
| `Esc` | Back to main view |

### Debug View

| Key | Action |
|-----|--------|
| `Tab` | Switch panels (source/callstack/variables/console) |
| `F5` | Continue |
| `F9` | Toggle breakpoint |
| `F10` | Step over |
| `F11` | Step into |
| `Shift+F11` | Step out |
| `Esc` | Exit debug mode |

### VI Mode (Debug Console)

| Key | Action |
|-----|--------|
| `i/a/A` | Enter insert mode |
| `Esc` | Normal mode |
| `h/l` | Left/right |
| `w/b/e` | Word navigation |
| `0/$` | Start/end of line |
| `d{motion}` | Delete |
| `c{motion}` | Change |
| `y{motion}` | Yank |
| `p/P` | Paste after/before |
| `u` | Undo |
| `/pattern` | Search forward |
| `n/N` | Next/previous match |

## Debugging CF Workers

wrkmn includes a full CDP-based debugger for Cloudflare Workers:

1. Give each worker a distinct `inspector_port` in `worker-compose.yaml` (default wrangler port is 9229). wrkmn forwards this to `wrangler dev --inspector-port`, so multiple workers can be debugged at once without colliding.
2. Start the worker (`d`) — it must be running to attach.
3. Select it and press `D` to attach the debugger.
4. Set breakpoints with `F9`, step with `F10` (over) / `F11` (into) / `F12` (out), continue with `F5`.
5. `Tab` cycles panels (source / call stack / variables / console).
6. Inspect variables in the variables panel and evaluate expressions in the REPL console.
7. Press `Esc` (from any panel except the console) to detach and return to the dashboard.

The inspector is a standard CDP endpoint, so you can also attach external clients (Chrome DevTools, VS Code) to the same `inspector_port`.

## Themes

Built-in themes with optional transparent backgrounds:

- **default** - Cool blues
- **dracula** - Purple and pink
- **catppuccin** - Pastel colors (Mocha variant)
- **nord** - Arctic blue

Enable transparency to let your terminal background show through:

```bash
./wrkmn --theme nord --transparent
```

Or in config:

```yaml
ui:
  theme: nord
  transparent_background: true
```

## Requirements

- Go 1.21+
- For worker debugging: Workers running with `wrangler dev --inspector-port`

## License

MIT
