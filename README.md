# wrkmn

A powerful TUI for managing Cloudflare Workers and related processes. Think of it as process-compose meets k9s, specifically designed for CF Worker development.

## Features

- **Auto-discovery** - Automatically discovers Cloudflare Workers by scanning for `cloudflare.config.{ts,mts,js,mjs}`; directories with only a Wrangler config are flagged as unmigrated with a `cf migrate` hint
- **Process orchestration** - Manage workers alongside services like Docker, databases, etc.
- **Dependency management** - Define startup order with health check conditions
- **Health checks** - HTTP, TCP, and exec-based checks gate startup ordering
- **Live logs** - Real-time log streaming with viewport scrolling
- **Integrated debugger** - CDP-based debugging for CF Workers (breakpoints, stepping, REPL)
- **VI mode** - Bash-style vi editing in the debug console
- **Themes** - Built-in themes (default, dracula, catppuccin, nord) with transparency support

## Installation

Prebuilt binaries for Linux and macOS (amd64 and arm64) are attached to each
[release](https://github.com/jlangston/wrkmn/releases). Download the archive
for your platform, then:

```bash
tar -xzf wrkmn_*.tar.gz
install wrkmn_*/wrkmn ~/.local/bin/   # or anywhere on PATH
```

Windows is not supported (process management relies on POSIX process groups).

Releases are built by GitHub Actions when a `v*` tag is pushed:

```bash
git tag v1.0.0 && git push origin v1.0.0
```

Or build from source:

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

# Drive a running wrkmn from a shell (see "Remote control (agents)")
./wrkmn ctl status
./wrkmn ctl start stretch-ui
```

### CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file (default: worker-compose.yaml) |
| `--theme` | `-t` | Color theme: default, dracula, catppuccin, nord |
| `--transparent` | | Use terminal's background color |
| `--no-discovery` | | Disable auto-discovery of workers |
| `--skip-preflight` | | Skip startup pre-flight environment checks |
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

### Process Types

- `worker` - Cloudflare Worker launched via `cf dev`; the debugger attaches automatically
- `service` - Generic process (docker, database, etc.)
- `oneshot` - runs once; exit 0 marks it completed; dependents can wait on it with `condition: process_completed`

### Restart Policy

```yaml
defaults:
  restart:
    enabled: true     # restart worker/service processes that crash
    max_tries: 3      # consecutive crash restarts before giving up (default 3)
    delay: 1s         # wait between restarts (default 1s)
```

Applies to config-defined `worker`/`service` processes (a process's own `restart:` overrides `defaults.restart`). A run lasting 10s+ resets the crash counter, and manual stops never trigger a restart. `oneshot` processes are never restarted.

### Dev Modes

Each worker runs in one of two modes (press `m` to switch; set a default with `dev_mode:` in `worker-compose.yaml`):

| Mode | `cf dev` | Bindings | Debugger |
| --- | --- | --- | --- |
| `local` | `--local` | all local simulations | ✅ |
| `remote-bindings` (default) | _(none)_ | honors each worker's `remote: true` bindings | ✅ |

Full-remote dev doesn't exist in the cf CLI, so it isn't an option here.

### Modes

A worker's `cloudflare.config.ts` can export multiple named configs (e.g. `development`, `production`) that `cf dev --mode <name>` selects between. List the ones a worker supports under `modes:` in `worker-compose.yaml` (defaults to `[development, production]` for `type: worker` processes), then press `e` to pick one. Leaving the selection empty means wrkmn passes no `--mode` flag at all, so cf falls back to its own default (`development`).

Dev mode (`m`) and mode (`e`) selections persist across sessions in
`.wrkmn-state.json` next to your config — add it to your project's .gitignore.
Older state files that stored `selected_env` are migrated automatically.

### Dependency Conditions

- `process_started` - Wait for process to start
- `process_healthy` - Wait for health check to pass
- `process_completed` - Wait for a `oneshot` to exit 0

### Health Check Types

- `http` - HTTP GET returns 2xx
- `tcp` - TCP connection succeeds
- `exec` - Command exits with 0

Health checks gate startup ordering (dependents wait on `process_healthy`);
once start-all orchestration finishes, wrkmn does not keep monitoring health
in the background.

## Validation

```bash
wrkmn validate [directory]   # check config + environment, start nothing
```

Checks: YAML validity, dependency cycles, process types, dev modes, the `cf`
CLI on PATH (if any worker processes are configured), commands on PATH,
working directories, unmigrated Wrangler projects (missing `cf migrate`), and
env files (explicit = error, inherited default = warning). A configured
`inspector_port` is reported as a deprecated, ignored setting. Exit 0 unless
errors.

The TUI runs the same checks at startup: errors block, warnings pause for
Enter. Bypass with `--skip-preflight`.

## Keybindings

Every process-control key below has a `wrkmn ctl` equivalent that runs the
same handler (see [Remote control (agents)](#remote-control-agents)).

### Main View

| Key | Action |
|-----|--------|
| `j/k` | Navigate up/down |
| `Enter` / `l` | View logs |
| `d` | Start (dev) |
| `D` | Enter debug mode (running CF Workers only) |
| `m` | Select dev mode (local / remote-bindings) |
| `s` | Run a script |
| `r` | Remote logs (CF Workers) |
| `e` | Select mode |
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

## Remote control (agents)

A running wrkmn listens on a Unix control socket so scripts and coding
agents can drive it without a keyboard. `wrkmn ctl` is the client:

```bash
wrkmn ctl status                      # every row of the process table
wrkmn ctl start NAME                  # = pressing d on NAME
wrkmn ctl stop NAME                   # = pressing x on NAME
wrkmn ctl restart NAME                # = x then d
wrkmn ctl start-all                   # = pressing a (dependency order, health gating)
wrkmn ctl stop-all                    # = pressing z
wrkmn ctl logs NAME [--tail N]        # last N captured log lines (default 200)

# common flags
wrkmn ctl status --json               # raw {"ok":true,"result":...} response
wrkmn ctl status --config path/to/worker-compose.yaml   # target another project
wrkmn ctl status --dir /path/to/project
wrkmn ctl start-all --timeout 20m     # start-all returns when orchestration finishes
```

Requests are injected into the TUI as messages and handled by the same code
the keybindings use, so dev mode (`m`), selected mode (`e`), dependency
ordering, health gating and env-file resolution behave identically. If a key
would raise a y/N confirm dialog, `ctl` answers **yes**. `start`, `stop` and
`restart` reply once the launch/stop has been applied; `start-all` replies
when the scheduler finishes (its result is the full status snapshot).

**Finding the instance.** The socket path is derived from the project
directory (the one holding `worker-compose.yaml`), so run `ctl` from that
directory or point `--config`/`--dir` at it:

- `$XDG_RUNTIME_DIR/wrkmn/<id>.sock` when `XDG_RUNTIME_DIR` is set, else
  `/tmp/wrkmn-<uid>/<id>.sock`, where `<id>` is the first 12 hex chars of
  sha256 over the absolute (symlink-resolved) project directory.
- `WRKMN_CONTROL_SOCKET=<path>` overrides it on both the TUI and `ctl`.

The socket is created when the TUI starts (a stale file from a crash is
replaced; a live one is never clobbered) and removed on exit. With no
reachable instance, `ctl` exits 1 with `no running wrkmn for <dir>; socket
<path>`; an `ok:false` reply also exits 1.

**Protocol** (for clients other than `ctl`): one newline-terminated JSON
request per connection, one newline-terminated JSON response.
`{"cmd":"status"}`, `{"cmd":"start","name":"stretch-ui"}`,
`{"cmd":"stop","name":...}`, `{"cmd":"restart","name":...}`,
`{"cmd":"start-all"}`, `{"cmd":"stop-all"}`, `{"cmd":"logs","name":...,"tail":200}`
→ `{"ok":true,"result":...}` or `{"ok":false,"error":"..."}`. `status` rows
carry `name`, `status`, `port`, `uptime_seconds`, `path`, `type` and, for
workers, `dev_mode`, `selected_mode`, `dev_server_url` and `pid` while running.

## Debugging CF Workers

wrkmn includes a full CDP-based debugger for Cloudflare Workers, and it attaches automatically — there's nothing to configure:

1. Start the worker (`d`) — it must be running to attach.
2. Select it and press `D` to attach the debugger. wrkmn reads the `Debug:` URL off the worker's `cf dev` startup banner, fetches its `/__debug` page, and extracts the CDP websocket; if the banner hasn't printed yet, it falls back to scanning ports 9229–9249 for an inspector target matching the worker's name.
3. Set breakpoints with `F9`, step with `F10` (over) / `F11` (into) / `Shift+F11` (out), continue with `F5`.
4. `Tab` cycles panels (source / call stack / variables / console).
5. Inspect variables in the variables panel and evaluate expressions in the REPL console.
6. Press `Esc` (from any panel except the console) to detach and return to the dashboard.

**Known v1 limitation:** `cf dev` serves an SSR-transformed module (via the Cloudflare Vite plugin), which renames module-level bindings — `env` becomes something like `__vite_ssr_import_0__.env`. Evaluating a binding by its original name in the REPL throws a `ReferenceError`; frame-local variables are unaffected.

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
- The `cf` CLI on PATH for any `worker`-type process or discovered worker (`bun add --global cf`)

## Testing

```bash
go test ./...
```

To verify a machine's cf setup end-to-end — `cf` and `bun` on PATH plus
network access to scaffold a throwaway project — run the live smoke test. It
launches a worker through the process manager, attaches the debugger through
the same discovery path the TUI uses, and drives a breakpoint
pause/inspect/resume:

```bash
go test -tags cfsmoke ./test/integration/ -run TestCfLiveSmoke -v -timeout 300s
```

## License

MIT
