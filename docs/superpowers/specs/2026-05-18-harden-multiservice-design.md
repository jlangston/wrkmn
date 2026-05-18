# Design: Hardening wrkmn for Real Multi-Service Projects

Date: 2026-05-18
Status: Approved
Branch: `harden-multiservice`

## Problem

wrkmn's documented features all *exist* in code and the project builds and
tests pass. But running the actual config loader and process manager against a
real multi-service project (the Stretch platform: Docker + 6 Cloudflare
Workers, all dependent on a shared `.dev.vars` env file) exposes gaps that make
the tool unusable for that project. A purely static audit missed these because
the code compiles and looks complete.

### Verified gaps

| # | Issue | Evidence | Impact |
|---|-------|----------|--------|
| 1 | `env_file` / `env_files` never loaded | `internal/process/manager.go` `StartProcess` (and `StartWorker`) only apply `cfg.Environment`; `cfg.EnvFile`/`cfg.EnvFiles` are ignored | Every worker starts without `DATABASE_URL`, WorkOS, Stripe, JWT secrets → entire stack fails immediately |
| 2 | Process exit never tracked | `manager.go` `cmd.Wait()` goroutine's "process exited" branch is an empty no-op; no exit code captured anywhere | A failed one-shot/setup process is invisible; dependents launch against a broken state; one-shots display as "running" forever |
| 3 | No `oneshot` semantics | `internal/config/types.go` only defines `worker` and `service`; no `process_completed` condition | Run-once commands (`docker compose run --rm db_setup`, migrations) cannot express "runs once, exit 0 = done" |
| 4 | No non-interactive validation | Only `themes` and `version` subcommands; the only way to exercise config is launching the full TUI | Cannot test config without booting Docker + 6 workers |
| 5 | No pre-flight checks | `StartProcess` execs directly and fails with cryptic child-process errors | Missing `bun`/`docker`, missing `.dev.vars`, occupied inspector port surface as opaque failures |

## Goals

- Make wrkmn correctly run a real multi-service project with shared env files,
  one-shot setup steps, and inter-service dependencies.
- Make wrkmn testable without launching services.
- Surface configuration and environment problems before they cause cryptic
  runtime failures.

## Non-Goals

- Replacing the project's `devenv`/`setup` workflow.
- Reworking the debugger, VI mode, themes, or UI beyond what these fixes
  require.
- Building stretch-types or otherwise reaching into the target project's build
  steps. wrkmn launches the commands it is configured with; it does not own the
  target project's prerequisites.

## Components

### Component 1 — Env file loading (`internal/env/`, new package)

New package `internal/env` with:

```go
// Load parses and merges dotenv files in order. Later files override earlier.
// Returns merged KEY=VALUE pairs suitable for appending to os.Environ()-derived
// env, plus the count of files actually read.
func Load(paths []string) (map[string]string, error)
```

Parsing rules (dotenv-compatible, matching what `wrangler`/`bun` expect for
`.dev.vars`):

- `KEY=VALUE` per line.
- Optional leading `export ` is stripped.
- Surrounding single or double quotes on the value are stripped; no escape
  expansion inside single quotes; `\n` etc. expanded inside double quotes.
- Lines that are blank or start with `#` are ignored.
- Inline `#` comments are **not** stripped (values may contain `#`); only
  whole-line comments.
- A missing file is an error returned to the caller (callers decide severity).

Manager wiring (`internal/process/manager.go`):

- Both `StartProcess` and `StartWorker` build `cmd.Env` as:
  1. Start from `os.Environ()`.
  2. Resolve each `env_file` / `env_files` path: relative paths resolved first
     against the process `working_dir`, then against `Config.BaseDir`. First
     existing wins.
  3. Merge file values (in declared order) on top of the OS env.
  4. Overlay the inline `Environment` map last (inline always wins).
- **Missing-file severity depends on provenance** (verified necessary against
  the target project: `defaults.env_file: .dev.vars` applies to all processes,
  but `postgres`, `db-setup`, `stretch-mcp`, `stretch-player` have no
  `.dev.vars` in their working dir):
  - An **explicitly set** per-process `env_file`/`env_files` that cannot be
    resolved → `StartProcess` returns an error (the process does not start with
    a silently missing environment it explicitly asked for).
  - An env file **inherited from `defaults`** that cannot be resolved → skipped
    silently (best-effort); the process starts. (For `stretch-mcp`/`player` the
    root `.dev.vars` still resolves via the BaseDir fallback, so they get the
    shared vars anyway; only the truly-absent case is skipped.)
- Provenance tracking: `applyDefaults` currently overwrites `Process.EnvFile`
  with the default string, erasing the distinction. The loader will instead set
  a non-YAML field `Process.envFileFromDefault bool` (or equivalent: only fill
  `EnvFile`/`EnvFiles` from defaults when the process left them empty *and*
  record that it was defaulted). The manager reads this flag to choose error
  vs. skip. Pre-flight (Component 5) applies the same rule: explicit missing =
  error, defaulted missing = warning.

This package is pure and unit-tested with table tests (quoting, comments,
export prefix, override order, missing file).

### Component 2 — Exit tracking (`internal/process/manager.go`)

Extend `Process`:

```go
type ProcessRunState int
const (
    RunStateRunning ProcessRunState = iota
    RunStateExited   // exited on its own
    RunStateKilled   // we cancelled/stopped it
)

// added fields (guarded by p.mu)
runState ProcessRunState
exitCode int
exitErr  error
finished time.Time
```

The `cmd.Wait()` goroutine:

- On natural exit: capture exit code (0 if `err == nil`, else from
  `*exec.ExitError`), set `runState = RunStateExited`, `finished = now`, store
  `exitErr`.
- On context cancellation (we stopped it): set `runState = RunStateKilled`.

Add accessors: `RunState()`, `ExitCode()`, `ExitErr()`, `Finished()`. No
behavior change to scheduler yet — this is the truthful-state foundation.

### Component 3 — `oneshot` process type (config + scheduler + dependency)

`internal/config/types.go`:

- `ProcessTypeOneshot ProcessType = "oneshot"`
- `ConditionCompleted DependencyCondition = "process_completed"`

`internal/process/dependency.go`: new terminal state `StateCompleted` (ordered
above `StateRunning` so existing `>= StateRunning` checks for `process_started`
still pass for a completed one-shot).

`internal/process/scheduler.go` `startProcess`:

- For `type: oneshot`: after `starter(...)` returns, **wait for the process to
  exit** (poll `proc.RunState()` with the same ctx/timeout pattern already used
  by `waitForDependency`).
  - Exit 0 → `UpdateState(name, StateCompleted)`, `UpdateHealth(name,
    HealthHealthy)`, emit a `completed` event. Satisfies both
    `process_started` and `process_completed`.
  - Non-zero exit → `UpdateState(name, StateFailed)`, emit `failed` with the
    exit code in the message; dependents waiting on it fail fast.
- `service` / `worker` behavior is unchanged: they are expected to stay up;
  exiting maps to `StateStopped`/`StateFailed` as before. Backward compatible.

`waitForDependency`: add a `ConditionCompleted` case (satisfied when dep node
is `StateCompleted`; fails if dep is `StateFailed`).

Loader validation (`internal/config/loader.go`):

- `oneshot` with a `health_check` → validation error (a process that exits
  cannot be health-monitored the same way).
- A dependency using `process_completed` whose target is not `type: oneshot` →
  validation error.
- Unknown `type` value → validation error (currently unvalidated).

### Component 4 — `wrkmn validate` command (`cmd/validate.go`, new)

`wrkmn validate [directory] [-c config]`:

1. Load config (same loader path as the TUI).
2. Apply defaults, resolve working dirs.
3. Build the dependency graph (surfacing cycle errors).
4. Run pre-flight checks (Component 5) in report mode.
5. Print a readable, grouped report.
6. Exit `0` if no errors, non-zero if any error-severity issue (warnings do not
   fail the exit code).

Launches zero services. This is the primary non-interactive test surface.

### Component 5 — Pre-flight checks (`internal/preflight/`, new package)

```go
type Severity int
const ( SeverityWarning Severity = iota; SeverityError )

type Issue struct {
    Severity Severity
    Process  string // "" for config-level
    Message  string
}

func Check(cfg *config.Config) []Issue
```

Checks:

- Command resolvable on `PATH` (`exec.LookPath`) — error if not.
- `working_dir` exists and is a directory — error if not.
- Each `env_file`/`env_files` resolves and is readable, using the same
  provenance rule as Component 1: an unresolved **explicit** env file is an
  error; an unresolved **defaulted** env file is a warning.
- `inspector_port` (worker type) not already bound (short TCP dial) — warning.
- Duplicate `inspector_port` across processes — error.

Consumed by:

- `wrkmn validate` — prints and sets exit code.
- TUI startup — **blocking summary screen**: render all issues before any
  process starts. Hard errors require quit; warnings require an explicit
  keypress to proceed. (Chosen over toast-and-continue so the user sees
  problems before 6 services thrash.)

## Target project config update

After the code changes land, update the Stretch project's
`worker-compose.yaml` (kept backward compatible for all other configs):

- `db-setup`: `type: service` → `type: oneshot`.
- `stretch-platform`'s dependency on `db-setup`: `condition:
  process_started` → `condition: process_completed` (so the platform waits for
  migrations to *finish* rather than merely *start*).

No other config changes required; all existing keys already map to real
struct fields (verified).

## Data Flow (after changes)

1. `cmd/root.go` or `cmd/validate.go` loads config, applies defaults, resolves
   dirs.
2. Loader validation rejects bad `type`/condition/health-check combinations.
3. Pre-flight runs. `validate` reports + exits; TUI shows blocking summary.
4. Scheduler builds graph, starts by dependency level.
5. `StartProcess` loads env files (working_dir → BaseDir resolution), merges
   with OS env and inline `Environment`, then execs.
6. `cmd.Wait()` goroutine records real exit state.
7. For `oneshot`, scheduler waits for exit: 0 → `StateCompleted` (satisfies
   `process_started` and `process_completed`); non-zero → `StateFailed`,
   dependents fail fast.

## Testing Strategy

- `internal/env`: table-driven unit tests (quoting, `export`, comments,
  override order, missing file, CRLF).
- `internal/preflight`: unit tests with temp dirs / fake PATH for each issue
  type and severity.
- Config loader: tests for new validation errors (oneshot+healthcheck,
  process_completed against non-oneshot, unknown type).
- Scheduler: test that a oneshot exiting 0 reaches `StateCompleted` and
  satisfies a `process_completed` dependent; that non-zero exit fails the
  dependent.
- Manager: test exit-code capture for exit 0, exit non-zero, and
  cancellation (RunStateKilled).
- Manual/interactive verification by the user against the real Stretch
  `worker-compose.yaml`; iterate on failures reported back.

## Risks & Mitigations

- **Env parsing divergence from wrangler/bun.** Mitigation: match the common
  dotenv subset `.dev.vars` actually uses; do not over-engineer escape
  handling; cover with tests; the user verifies against the real file.
- **Blocking pre-flight screen annoyance.** Mitigation: only hard errors force
  quit; warnings are a single keypress.
- **`oneshot` wait could hang if a "one-shot" never exits.** Mitigation: reuse
  the existing bounded ctx/timeout pattern from `waitForDependency`; timeout →
  `StateFailed` with a clear message.
- **Static-audit blind spot recurring.** Mitigation: every component has
  executable tests; final sign-off is the user running the real stack, not a
  code read.

## Out of Scope / Follow-ups

- Restart/backoff behavior for `oneshot` (currently restart policy is
  service/worker oriented).
- A `process_completed` timeout configurable per dependency.
- Discovered (auto-detected) workers gaining `oneshot` classification.
