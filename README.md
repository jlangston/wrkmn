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
| `Enter` | Start/stop selected process |
| `l` | View logs |
| `d` | Enter debug mode (workers only) |
| `a` | Start all |
| `x` | Stop all |
| `r` | Refresh |
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

1. Ensure your worker has an `inspector_port` configured (default: 9229)
2. Start the worker
3. Press `d` to enter debug mode
4. Set breakpoints with `F9` or `b`
5. Use stepping controls to navigate through code
6. Inspect variables in the variables panel
7. Evaluate expressions in the REPL console

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
