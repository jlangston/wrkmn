# Per-worker dev mode (local / remote-bindings / full-remote)

Date: 2026-06-18
Status: Design (approved for spec review)

## Problem

wrkmn always launches workers with a plain local `wrangler dev` (plus an
appended `--inspector-port` for its CDP debugger). There is no way to run a
worker against real Cloudflare resources, or fully on Cloudflare's edge, from
the TUI. Developers who want "remote" behavior have to drop out of wrkmn and run
wrangler by hand — losing the orchestration, logs, and debugger.

The request: make running remotely a first-class, per-worker option, while being
honest about which modes still support the in-TUI debugger.

## Background: what "remote" means in current Wrangler

There are two distinct, non-interchangeable notions of "remote", and they differ
precisely on whether breakpoint debugging works:

- **Remote bindings** (GA since Wrangler v4.37.0): the worker's **code still runs
  locally** in workerd, but individual bindings marked `remote: true` in that
  worker's own wrangler config proxy their calls to the deployed Cloudflare
  resource (D1, KV, R2, Hyperdrive, Queues, …). Because code runs locally, the
  V8 inspector — and therefore wrkmn's debugger, breakpoints, stepping, and REPL
  — **keep working**. There is **no** blanket CLI flag to force all bindings
  remote; `remote: true` is set per binding in config. The only related
  `wrangler dev` switch is `--local`, which *disables* all remote bindings for
  the session (forces everything local).
- **Full remote dev** (`wrangler dev --remote`, now labelled **Legacy** by
  Cloudflare): the worker's code is uploaded and **executes on Cloudflare's
  edge**. All bindings are remote. Iteration is slower (upload per change), a
  50-routes-per-zone limit applies, and because the code is not in local
  workerd, wrkmn's inspector-based breakpoint debugging **cannot attach**.

### Mode → `wrangler dev` mapping

| wrkmn mode        | appended flags                          | bindings                              | debugger |
| ----------------- | --------------------------------------- | ------------------------------------- | -------- |
| `local`           | `--inspector-port <p> --local`          | all forced to local simulations       | ✅       |
| `remote-bindings` | `--inspector-port <p>`                  | honors worker's own `remote: true`    | ✅       |
| `remote`          | `--remote` (no inspector port)          | all remote (code on edge)             | ❌       |

`remote-bindings` is plain `wrangler dev` and is identical to wrkmn's current
behavior, so it is the default when nothing is configured (no behavior change for
existing setups).

## Decisions (from brainstorming)

- Offer all three modes as selectable, per worker. Surface debug capability in UI.
- Mode is set by a **config default** (`dev_mode:`) plus a **runtime keybinding**
  (`m`) that opens a selector, mirroring the existing `e` env selector.
- Default mode: `remote-bindings` (preserves today's behavior).
- Safety guard: **confirm only for full-remote** before starting. `local` and
  `remote-bindings` start without friction.
- wrkmn does **not** rewrite worker configs to force bindings remote (out of
  scope). `remote-bindings` means "honor whatever the worker config declares".
- Fold in the adjacent `--env` gap: the selected env is currently used only for
  tail logs / account ID, not passed to `wrangler dev`. The new passthrough
  helper will also emit `--env <name>` so the `e` selector affects the dev run.

## Design

### 1. Data model & config

- New type in `internal/config`:
  ```go
  type DevMode string
  const (
      DevModeLocal          DevMode = "local"
      DevModeRemoteBindings DevMode = "remote-bindings"
      DevModeRemote         DevMode = "remote"
  )
  ```
  Chosen home is the `config` package because it backs a YAML field and neither
  `config` nor `worker` currently imports the other (both are near-leaves;
  `config → theme`, `worker → wrangler`). `worker` will import `config` for the
  type; verified no cycle (`config` does not import `worker`).
- `config.Process` gains `DevMode DevMode \`yaml:"dev_mode,omitempty"\``.
- `worker.Worker` gains a runtime `DevMode DevMode` field, alongside the existing
  `SelectedEnv`. Initialized from the process config default (or
  `DevModeRemoteBindings` when unset).
- Loader (`internal/config/loader.go`): validate `dev_mode` against the three
  constants (error on unknown value), apply the `remote-bindings` default when
  empty.

### 2. Command construction

Replace the two ad-hoc `appendInspectorFlag` call sites
(`StartWorker`, `StartProcess` in `internal/process/manager.go`) with one helper:

```go
// buildWranglerPassthrough returns the args appended after the `bun run <script>`
// (or configured command) invocation, using a single `--` separator so they
// reach the underlying `wrangler dev`.
func buildWranglerPassthrough(mode config.DevMode, inspectorPort int, env string) []string
```

Rules:
- Emit exactly one `--` separator (two would break passthrough — the current bug
  risk if mode and inspector were appended independently).
- `local`        → `-- --inspector-port <p> --local`
- `remote-bindings` → `-- --inspector-port <p>`
- `remote`       → `-- --remote` (omit `--inspector-port`; debugging unavailable)
- Append `--env <env>` when `env` is non-empty (applies to all modes).
- `--inspector-port` only included when `inspectorPort > 0` and mode supports
  debugging.

Both `StartWorker` and `StartProcess` call this helper.

### 3. TUI

- **`ModeSelector` popup** (`internal/ui/mode_selector.go`), cloned from
  `EnvSelector`: opened with `m`, lists the three modes, each annotated with
  `debug: ✅/❌` and `real resources: yes/no`. Selecting sets `worker.DevMode`.
  Closing without selection is a no-op.
- **Table** (`internal/ui/table.go`): show the active mode as a badge plus a
  debug-capable indicator. Add `m: mode` to the footer hint line.
- **Debug gating**: in `remote` mode, the `D` debug action shows a toast —
  "Debugging unavailable in full-remote mode (code runs on Cloudflare's edge)" —
  instead of attaching.
- **Full-remote start guard**: starting a worker whose mode is `remote` first
  shows a y/n confirmation. `local`/`remote-bindings` start immediately. The
  guard lives at the start-action handler so it covers both `d` (single) and `a`
  (start all) paths. For "start all", show a single batched confirmation that
  lists every worker resolving to `remote` mode; confirming starts them all,
  cancelling skips only those (non-remote workers still start).

### 4. Caveats surfaced to the user

For `remote` mode, the selector (and README) note:
- No breakpoint debugging.
- Slower iteration (upload per change).
- 50-routes-per-zone limit.
- **Deployed secrets are not loaded** in remote dev for this stack; local
  `.dev.vars` still apply. (Known stack-specific caveat.)

## Testing

- `buildWranglerPassthrough`: table test — each mode + env + inspector-port
  combination → expected argv; assert single `--`, inspector omitted in `remote`,
  `--env` appended when set.
- Config loader: parse valid `dev_mode`, reject unknown, default to
  `remote-bindings` when empty.
- Mode-cycle / selection reducer: selecting a mode updates `worker.DevMode`;
  cancel is a no-op.

Follows the existing Go `_test.go` patterns in `internal/process` and
`internal/wrangler`.

## Out of scope

- Rewriting worker wrangler configs to force bindings remote (rejected — that
  belongs in each worker's own config).
- Any change to the remote-logs (`r`) or env-selector (`e`) behavior beyond
  wiring `--env` into the dev passthrough.
