# Design: cf-only Cutover (wrangler → cf CLI)

Date: 2026-10-01
Status: Draft (pending user review)
Branch: `cf-cutover`

## Problem

Cloudflare's new `cf` CLI (v1.0.0-beta, Sept 2026) replaces Wrangler for Workers
development. On this machine, global wrangler is already gone and `cf` is
installed. wrkmn is wrangler-shaped end to end: it discovers workers by
`wrangler.toml`, parses that TOML for ports/environments/account IDs, launches
dev sessions by appending wrangler passthrough flags (`--inspector-port`,
`--local`, `--remote`, `--env`) after `--`, and attaches its debugger to raw
inspector ports.

**User decisions (2026-10-01, in chat):**

1. **cf-only cutover.** The wrangler launch path is removed entirely, not
   dual-supported. Unmigrated wrangler projects (including all of stretch
   today) stop being manageable by wrkmn until they are `cf migrate`d.
2. **The in-TUI debugger keeps working at any cost — TUI only.** No
   browser-DevTools fallback. Attach failures surface as TUI errors to fix,
   not as degraded modes.

## Probe findings (ground truth, verified 2026-10-01)

Probed with `cf` 1.0.0-beta.10, `@cloudflare/vite-plugin` 2.0.0-beta, Vite 8.3,
via a scaffolded `cf init` project (throwaway, deleted):

- `cf dev` delegates to Vite and prints a banner:
  `Local: http://localhost:5173/` and `Debug: http://localhost:5173/__debug`.
- `GET <local>/__debug` returns HTML embedding the CDP websocket:
  `ws=localhost%3A9229%2F<worker-name>` → `ws://localhost:9229/<worker-name>`.
- The inspector port serves classic DevTools discovery: `GET
  http://localhost:9229/json` → `[{"webSocketDebuggerUrl":
  "ws://localhost:9229/<worker-name>", ...}]`.
- Raw CDP handshake verified compatible with wrkmn's engine:
  `Debugger.enable` → OK with debuggerId; `Debugger.scriptParsed` events
  stream; `Runtime.enable` → OK.
- Concurrent `cf dev` instances auto-increment both ports (Vite 5173→5174,
  inspector 9229→9230) with per-instance `/json`. No collision handling needed
  beyond discovering each worker's own ports.
- Bonus (not used in v1): the Vite plugin exposes a Local Explorer API at
  `<local>/cdn-cgi/local/explorer/api` (local workers+bindings listing,
  read-only SQL over captured spans/logs). Noted as a follow-up opportunity
  for log/metrics integration.

Also verified from Cloudflare docs:

- `cf dev`/`cf build`/`cf deploy` read only `cloudflare.config.ts` — never
  `wrangler.toml`. `cf migrate` converts projects.
- In framework-run projects `cf dev` forwards only `--mode` and rejects other
  arguments (`--port`, `--inspector-port` fail). `--local` is a cf-level flag
  and is accepted. Ports are configured in `vite.config.ts`.
- Wrangler's `--env` concept becomes `--mode` (default `development` for dev).
  Modes are inputs to a config *function* in `cloudflare.config.ts`, so they
  cannot be statically enumerated.
- There is no full-remote dev (`wrangler dev --remote` has no cf equivalent).

## Goals

- wrkmn discovers, launches, debugs, and manages cf projects
  (`cloudflare.config.ts`) exclusively.
- The in-TUI debugger (breakpoints, stepping, variables, REPL) works against
  `cf dev` sessions, TUI-only.
- Unmigrated wrangler projects are detected and reported clearly, not
  silently ignored.

## Non-Goals

- Dual wrangler/cf support (explicitly rejected by user).
- Running `cf migrate` on user projects (separate task if requested).
- A full-remote dev mode (does not exist in cf).
- Local Explorer API integration (follow-up).
- Statically parsing `cloudflare.config.ts` (it is TypeScript, possibly a
  function of mode; approach rejected during design).

## Approach: runtime-first metadata (Approach A, user-approved)

Discovery only checks that `cloudflare.config.ts` exists. Worker name comes
from `package.json` name or the directory name. The dev-server port and debug
URL are parsed from `cf dev`'s own stdout banner at runtime. Account ID (for
remote log tailing) comes from `CLOUDFLARE_ACCOUNT_ID`, worker-compose
configuration, or opportunistically from Build Output
(`.cloudflare/output/v0/config.json`) when a build has produced one. Modes are
declared per worker in `worker-compose.yaml`; wrkmn does not guess them.

## Components

### Component 1 — `internal/cfproject` (new package; replaces `internal/wrangler`)

All cf-specific knowledge lives here:

```go
// Detect reports whether dir is a cf project and returns the config path.
// Recognizes cloudflare.config.ts (primary), .js, .mjs, .mts.
func Detect(dir string) (configPath string, ok bool)

// DetectUnmigrated reports a wrangler-config-only directory (wrangler.toml /
// wrangler.json / wrangler.jsonc present, no cloudflare.config.*).
func DetectUnmigrated(dir string) bool

// BuildOutput is the subset of .cloudflare/output/v0/config.json wrkmn reads.
type BuildOutput struct {
    WorkerName string
    AccountID  string
}

// ReadBuildOutput loads Build Output if present. Missing file is (nil, nil):
// enrichment is strictly opportunistic. The format is beta; parse leniently
// (unknown fields ignored, missing fields empty).
func ReadBuildOutput(dir string) (*BuildOutput, error)

// Banner holds what cf dev / vite print at startup.
// ParseBannerLine incrementally matches "Local: <url>" and
// "Debug: <url>" lines (ANSI codes stripped); returns ok=false otherwise.
type Banner struct { LocalURL, DebugURL string }
func ParseBannerLine(line string) (kind BannerKind, url string, ok bool)

// ExtractDebugWS fetches debugURL and extracts the CDP websocket from the
// embedded "ws=" parameter (URL-decoded, "ws://" scheme added).
func ExtractDebugWS(ctx context.Context, debugURL string) (wsURL string, err error)
```

`internal/wrangler` (TOML parser, types) is deleted. Everything that imported
it is reworked (see Components 2 and 5).

### Component 2 — Discovery rework (`internal/worker`)

- A discovered worker is a directory containing `cloudflare.config.*`
  (detected via `cfproject.Detect`). `package.json` is no longer required;
  when present it supplies the name and scripts (the `s` scripts popup keeps
  working).
- Directories with only wrangler config files are NOT workers. Discovery
  counts them and the TUI/preflight surface them (Component 6).
- `Worker` struct changes:
  - Remove: `WranglerConfig`, `Environments`, `AccountID` (toml-derived),
    `GetAccountIDForEnv`, `GetCurrentAccountID` (replaced below),
    `InspectorPort` (no longer used to launch; the debugger discovers its own
    endpoint).
  - Rename: `SelectedEnv` → `SelectedMode` (persisted state key becomes
    `selected_mode`; `state.Apply` also reads the old `selected_env` key once
    for migration).
  - Add: `Modes []string` (from compose; default `["development",
    "production"]`), `DevServerURL string`, `DebugURL string` (runtime,
    written by the banner parser), `AccountID string` (resolution order:
    compose per-process `account_id` → Build Output → `CLOUDFLARE_ACCOUNT_ID`
    env).
  - `IsCloudflareWorker()` → true when a cf config was detected.
  - `EffectiveDevMode`/`SupportsDebugging`: `SupportsDebugging` returns true
    unconditionally (full-remote is gone); keep the method so UI call sites
    survive, or inline-remove — implementer's choice, no behavioral gate.

### Component 3 — Launch rework (`internal/process`)

- `buildWranglerPassthrough` is replaced by:

```go
// buildCfDevArgs returns args appended to a worker launch command.
// No "--" separator: `cf dev` takes these directly, and `bun run <script>`
// forwards trailing args to the script.
//   local           -> --local
//   remote-bindings -> (nothing; cf dev default)
//   mode != ""      -> --mode <mode>
func buildCfDevArgs(devMode config.DevMode, mode string) []string
```

- `config.DevModeRemote` is removed. Loader validation rejects
  `dev_mode: remote` with: `full-remote dev does not exist in the cf CLI;
  use remote-bindings (default) or local`.
- Discovered workers launch as `cf dev` (command `cf`, args
  `dev` + buildCfDevArgs) in the worker's directory — replacing the
  `bun run <script>` default. The `s` popup still runs arbitrary
  package.json scripts unchanged.
- Compose processes with `type: worker` keep their configured
  `command`/`args`; wrkmn appends `buildCfDevArgs(...)` directly (works for
  both `cf dev` commands and `bun run dev` scripts whose `dev` wraps
  `cf dev`).
- The full-remote confirm dialog, the `D`-key debug gate, and the
  start-all full-remote batch confirm are removed (`internal/ui/confirm.go`
  stays — it is generic — but its full-remote call sites go).
- **Banner parsing:** the manager's `readLogs` goroutine additionally feeds
  each line through `cfproject.ParseBannerLine` for worker processes and
  writes `Worker.DevServerURL`/`DebugURL`/`Port` when matched (same
  unsynchronized Worker-field write pattern as Status — see parked debt note
  in Risks).

### Component 4 — Modes (config + UI)

- New compose field per process: `modes: [development, staging, production]`
  (optional; default `["development", "production"]`). Validation: non-empty
  strings, no duplicates.
- The `e` selector becomes the **mode selector** (UI labels say "mode";
  keybinding unchanged). Selecting a mode sets `SelectedMode`; empty means
  "cf default" (development) and `--mode` is omitted.
- The `m` selector (dev mode) shrinks to two options: `local`,
  `remote-bindings` (default). Debug-availability column is dropped
  (always available).
- State persistence (`internal/state`): `selected_mode` replaces
  `selected_env`; dev-mode masking logic from the previous plan is preserved;
  `remote` values in stale state files are ignored by `Apply`'s validity
  switch (it already allowlists known modes — update the allowlist).

### Component 5 — Debugger attach (`internal/debug`)

Hard requirement: in-TUI only, no browser path.

- `inspector.go`'s port-probing discovery is replaced by a two-path discovery:
  1. **Primary:** wait (bounded: 30s, 250ms interval) for the worker's
     `DebugURL` (banner-parsed); call `cfproject.ExtractDebugWS`; dial the
     returned `ws://host:port/<name>` with the existing CDP session code.
  2. **Fallback:** scan inspector ports 9229–9249: `GET /json`, match an
     entry whose `webSocketDebuggerUrl` path equals `/<worker-name>`; dial it.
- Attach failures (no banner after timeout, HTTP error, no ws param, dial
  refused) produce a specific TUI status message naming the failed step.
  `D` never opens a browser.
- The CDP session/breakpoints/stepping/evaluation/source code is unchanged —
  the probe verified protocol compatibility (`Debugger.enable`,
  `scriptParsed`, `Runtime.enable` all behave as with wrangler).
- Source mapping caveat: Vite serves transformed modules; script URLs may be
  Vite-style (`/src/index.ts` or `file://` paths). `source.go`'s mapping gets
  verified against the live smoke (mandatory gate below) and adjusted there
  if paths differ — the spec does not guess beyond requiring the smoke to
  demonstrate: breakpoint set in the TUI → request pauses → variables visible
  → resume works.

### Component 6 — Preflight, validate, and messaging

- Pre-flight additions:
  - `cf` resolvable on PATH when any worker-type process exists — error.
  - Unmigrated wrangler project configured as `type: worker` or discovered →
    error at validate/preflight: `unmigrated Wrangler project (wrangler.toml)
    — run 'cf migrate' in <dir>`.
  - `inspector_port:` set in compose → warning: `inspector_port is ignored
    since the cf cutover; the debugger discovers its endpoint automatically`.
    (Field stays parseable for one release; docs drop it.)
- Pre-flight removals: the inspector-port occupancy and duplicate-port checks
  are retired with the field.
- The loader's type-inference heuristic "has inspector_port → type worker" is
  removed; untyped processes default to `service` as before, and worker-type
  processes must say `type: worker`.
- Dev-mode validation accepts `local`/`remote-bindings` only.

### Component 7 — Docs

README and CLAUDE.md: rewrite Dev Modes (two modes), Environments→Modes,
debugger section (auto-discovery via debug endpoint; no inspector_port),
discovery (`cloudflare.config.ts`), launch examples (`cf dev`), the sample
compose file, migration note for unmigrated projects, and the data-flow list.

## Data Flow (after changes)

1. Config load → validation (no `remote` dev mode, `modes` list checked).
2. Pre-flight: `cf` on PATH, unmigrated-project detection, env files, etc.
3. Discovery: directories with `cloudflare.config.*` become workers; names
   from package.json/dir; state restore (`selected_mode`, dev mode).
4. Launch (`d`/`a`): `cf dev` (or configured command) + `--mode`/`--local`;
   scheduler semantics unchanged.
5. `readLogs` parses the Vite banner → `DevServerURL`/`DebugURL`/`Port`
   populate live; table shows the real port.
6. `D`: banner-driven debug-ws extraction (fallback: /json scan) → existing
   CDP engine.
7. Remote logs (`r`): unchanged API tailing; account ID per Component 2
   resolution order; mode replaces env in the tail-name heuristic.

## Testing Strategy

- `internal/cfproject`: table tests — Detect/DetectUnmigrated (temp dirs),
  ParseBannerLine (real banner lines incl. ANSI-colored variants),
  ExtractDebugWS (httptest server serving the real captured `/__debug` HTML),
  ReadBuildOutput (lenient parsing, missing file).
- `internal/process`: buildCfDevArgs table test; banner-parse wiring test
  (fake process emitting a banner line → Worker fields populated).
- `internal/worker`: discovery tests reworked to cf configs; unmigrated
  detection test.
- `internal/config`/`internal/state`: mode validation, `remote` rejection,
  selected_mode migration from selected_env.
- `internal/debug`: fallback /json-scan matcher unit test (httptest).
- **Mandatory live smoke (gates completion):** scaffold a throwaway `cf init`
  project (scratchpad), run wrkmn against it: start (`d`), observe real port,
  attach debugger (`D`), set a breakpoint, curl the worker, observe pause +
  variables, resume, stop. The cutover is not done until this passes.
- Full suite + `-race` on touched packages, per repo convention.

## Risks & Mitigations

- **Beta surface drift** (banner text, `/__debug` HTML, ws param): two
  independent discovery paths (banner+HTML, /json scan); all cf-specific
  parsing centralized in `internal/cfproject` so drift is a one-package fix.
  Pin the probed versions in this spec for future archaeology.
- **Vite-style script URLs may break source mapping**: explicitly owned by
  the mandatory live smoke; `source.go` adjustments are in scope.
- **Worker.{DevServerURL,DebugURL,Port} written from the log goroutine**:
  same pre-existing unsynchronized-Worker-fields pattern already parked as
  architectural debt (see 2026-08-31 plan ledger rulings); do not expand
  synchronization scope here.
- **stretch unusable in wrkmn until migrated**: accepted by user with the
  cutover decision; preflight names each unmigrated directory so the
  migration TODO list is visible.
- **Multi-config dev sessions** (stretch's `dev:stack` two-worker wrangler
  session): no known cf equivalent yet; out of scope. Compose processes can
  still run any command, so a future equivalent slots in without wrkmn
  changes.

## Out of Scope / Follow-ups

- `cf migrate` of stretch (follow-up task on request).
- Local Explorer API integration (worker/bindings listing, observability SQL
  → could power richer wrkmn views later).
- Removing the deprecated `inspector_port` compose field (one release of
  warning first).
