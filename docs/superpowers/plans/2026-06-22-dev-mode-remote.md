# Per-worker dev mode (local / remote-bindings / full-remote) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let each worker in wrkmn run in one of three dev modes — `local`, `remote-bindings`, or `full-remote` — switchable at runtime, with the TUI surfacing which modes support the debugger.

**Architecture:** A `DevMode` enum lives in the `config` package, set via a `dev_mode:` config default and a runtime `worker.Worker.DevMode` field. A single `buildWranglerPassthrough` helper translates (mode, inspector port, env) into the flags appended after `--` to `wrangler dev`. The TUI gains an `m` mode selector (cloned from the existing env selector), a debug gate for full-remote, and a confirmation before starting a full-remote worker.

**Tech Stack:** Go, Bubble Tea / Bubbles / Lipgloss TUI, `go test` (standard library testing).

---

## ⚠️ Working-tree reconciliation (read first)

At plan-writing time the wrkmn working tree had **uncommitted in-progress work** (notably `go.mod`, `go.sum`, `internal/ui/console_view.go`, `internal/ui/logs.go`, `internal/vi/model.go`, new `internal/clipboard/`) on branch `remote-dev-mode`, plus a recent commit `201af50 "Progress on port detection and getting the debugger working"`. Before executing:

1. Run `git -C /home/jefflangston/dev/wrkmn status` and reconcile — line numbers in this plan may have drifted.
2. **Do not commit unless the user explicitly asks.** When a task says "Commit", scope it with an explicit `git add <files>` listing only that task's files, never `git add -A`.
3. The design spec is at `docs/superpowers/specs/2026-06-18-dev-mode-remote-design.md` (uncommitted) — keep it as the source of truth.

## File structure

| File | Responsibility | Change |
| --- | --- | --- |
| `internal/config/types.go` | `DevMode` type + constants; `dev_mode` field on `Process` | Modify |
| `internal/config/loader.go` | Validate `dev_mode`; default workers to `remote-bindings` | Modify |
| `internal/config/loader_test.go` | Tests for validation + default | Create/Modify |
| `internal/worker/types.go` | Runtime `DevMode` field + `EffectiveDevMode`/`SupportsDebugging` helpers | Modify |
| `internal/worker/types_test.go` | Tests for the two helpers | Create/Modify |
| `internal/process/manager.go` | `buildWranglerPassthrough`; wire into `StartWorker`/`StartProcess` | Modify |
| `internal/process/passthrough_test.go` | Tests for the flag builder | Create |
| `internal/ui/mode_selector.go` | `ModeSelector` popup component | Create |
| `internal/ui/confirm.go` | Minimal inline confirm dialog state | Create |
| `internal/ui/table.go` | Key `m`, selector wiring, debug gate, confirm, footer/badge | Modify |
| `README.md` / `CLAUDE.md` | Document the modes + caveats | Modify |

---

### Task 1: `DevMode` type, config field, validation, and default

**Files:**
- Modify: `internal/config/types.go` (Process struct, ~line 36)
- Modify: `internal/config/loader.go` (`validate` ~line 106, `applyDefaults` ~line 213)
- Create: `internal/config/loader_test.go` (append if it exists)

- [x] **Step 1: Write the failing tests**

Create `internal/config/loader_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "worker-compose.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestDevModeDefaultsToRemoteBindingsForWorkers(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: worker
    inspector_port: 9229
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Processes["api"].DevMode; got != DevModeRemoteBindings {
		t.Fatalf("DevMode = %q, want %q", got, DevModeRemoteBindings)
	}
}

func TestDevModeExplicitValuePreserved(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: worker
    dev_mode: local
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Processes["api"].DevMode; got != DevModeLocal {
		t.Fatalf("DevMode = %q, want %q", got, DevModeLocal)
	}
}

func TestDevModeInvalidRejected(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: worker
    dev_mode: bogus
`)
	if _, err := NewLoader(path).Load(); err == nil {
		t.Fatal("expected error for invalid dev_mode, got nil")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run TestDevMode -v`
Expected: compile error (`DevModeRemoteBindings` undefined) or FAIL.

- [x] **Step 3: Add the type and field**

In `internal/config/types.go`, after the `ProcessType` constants block (~line 67), add:

```go
// DevMode selects how a worker's `wrangler dev` connects to resources.
type DevMode string

const (
	// DevModeLocal forces all bindings to local simulations (wrangler dev --local).
	DevModeLocal DevMode = "local"
	// DevModeRemoteBindings runs code locally but honors the worker's own
	// `remote: true` bindings (plain wrangler dev). Debugger works.
	DevModeRemoteBindings DevMode = "remote-bindings"
	// DevModeRemote runs code on Cloudflare's edge (wrangler dev --remote).
	// No local breakpoint debugging.
	DevModeRemote DevMode = "remote"
)
```

In the `Process` struct (~line 36), add the field next to `InspectorPort`:

```go
	DevMode       DevMode           `yaml:"dev_mode,omitempty"`
```

- [x] **Step 4: Add validation**

In `internal/config/loader.go`, inside `validate` before the final `return nil` (~line 151), add:

```go
	// Validate dev_mode
	for name, proc := range cfg.Processes {
		switch proc.DevMode {
		case "", DevModeLocal, DevModeRemoteBindings, DevModeRemote:
			// Valid (empty defaults later)
		default:
			return fmt.Errorf("process %q: invalid dev_mode %q (must be local, remote-bindings, or remote)", name, proc.DevMode)
		}
	}
```

- [x] **Step 5: Apply the default**

In `internal/config/loader.go`, inside `applyDefaults`, immediately after the `Set default process type` block sets `proc.Type` (~line 265), add:

```go
		// Default workers to remote-bindings (today's plain `wrangler dev` behavior)
		if proc.Type == ProcessTypeWorker && proc.DevMode == "" {
			proc.DevMode = DevModeRemoteBindings
		}
```

(Place it before `proc.Source = SourceConfig` and `cfg.Processes[name] = proc`.)

- [x] **Step 6: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run TestDevMode -v`
Expected: PASS (all three).

- [x] **Step 7: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/config/types.go internal/config/loader.go internal/config/loader_test.go
git commit -m "config: add dev_mode field with validation and remote-bindings default"
```

---

### Task 2: Runtime `DevMode` on `worker.Worker` + helpers

**Files:**
- Modify: `internal/worker/types.go`
- Create: `internal/worker/types_test.go`

- [x] **Step 1: Write the failing tests**

Create `internal/worker/types_test.go`:

```go
package worker

import (
	"testing"

	"wrkmn/internal/config"
)

func TestEffectiveDevModeDefaultsToRemoteBindings(t *testing.T) {
	w := &Worker{}
	if got := w.EffectiveDevMode(); got != config.DevModeRemoteBindings {
		t.Fatalf("EffectiveDevMode() = %q, want %q", got, config.DevModeRemoteBindings)
	}
}

func TestSupportsDebugging(t *testing.T) {
	cases := []struct {
		mode config.DevMode
		want bool
	}{
		{"", true},
		{config.DevModeLocal, true},
		{config.DevModeRemoteBindings, true},
		{config.DevModeRemote, false},
	}
	for _, c := range cases {
		w := &Worker{DevMode: c.mode}
		if got := w.SupportsDebugging(); got != c.want {
			t.Fatalf("SupportsDebugging(%q) = %v, want %v", c.mode, got, c.want)
		}
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/worker/ -run "TestEffectiveDevMode|TestSupportsDebugging" -v`
Expected: compile error (`DevMode` field / methods undefined).

- [x] **Step 3: Add field + helpers**

In `internal/worker/types.go`, add the import and field. Update the import block:

```go
import (
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/wrangler"
)
```

In the `Worker` struct, after `SelectedEnv` (~line 28), add:

```go
	// DevMode is the runtime-selected dev mode (local / remote-bindings / remote).
	// Empty means remote-bindings (see EffectiveDevMode).
	DevMode config.DevMode `json:"dev_mode,omitempty"`
```

After `HasEnvironments` (~line 79), add:

```go
// EffectiveDevMode returns the selected dev mode, defaulting to remote-bindings.
func (w *Worker) EffectiveDevMode() config.DevMode {
	if w.DevMode == "" {
		return config.DevModeRemoteBindings
	}
	return w.DevMode
}

// SupportsDebugging reports whether breakpoint debugging is available in the
// current dev mode. Full-remote runs on the edge, so the local inspector cannot attach.
func (w *Worker) SupportsDebugging() bool {
	return w.EffectiveDevMode() != config.DevModeRemote
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/worker/ -run "TestEffectiveDevMode|TestSupportsDebugging" -v`
Expected: PASS. Also run `go build ./...` to confirm no import cycle.

- [x] **Step 5: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/worker/types.go internal/worker/types_test.go
git commit -m "worker: add runtime DevMode field and EffectiveDevMode/SupportsDebugging helpers"
```

---

### Task 3: `buildWranglerPassthrough` flag builder

**Files:**
- Modify: `internal/process/manager.go` (replace `appendInspectorFlag`, ~lines 19-30)
- Create: `internal/process/passthrough_test.go`

- [x] **Step 1: Write the failing tests**

Create `internal/process/passthrough_test.go`:

```go
package process

import (
	"reflect"
	"testing"

	"wrkmn/internal/config"
)

func TestBuildWranglerPassthrough(t *testing.T) {
	cases := []struct {
		name string
		mode config.DevMode
		port int
		env  string
		want []string
	}{
		{"remote-bindings with inspector", config.DevModeRemoteBindings, 9229, "",
			[]string{"--", "--inspector-port", "9229"}},
		{"empty mode treated as remote-bindings", "", 9229, "",
			[]string{"--", "--inspector-port", "9229"}},
		{"local adds --local", config.DevModeLocal, 9229, "",
			[]string{"--", "--inspector-port", "9229", "--local"}},
		{"remote omits inspector, adds --remote", config.DevModeRemote, 9229, "",
			[]string{"--", "--remote"}},
		{"env appended for all modes", config.DevModeRemoteBindings, 9230, "staging",
			[]string{"--", "--inspector-port", "9230", "--env", "staging"}},
		{"remote with env", config.DevModeRemote, 9229, "production",
			[]string{"--", "--remote", "--env", "production"}},
		{"no port no env remote-bindings yields nil", config.DevModeRemoteBindings, 0, "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildWranglerPassthrough(c.mode, c.port, c.env)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/process/ -run TestBuildWranglerPassthrough -v`
Expected: compile error (`buildWranglerPassthrough` undefined).

- [x] **Step 3: Replace `appendInspectorFlag` with the builder**

In `internal/process/manager.go`, replace the `appendInspectorFlag` function (~lines 19-30) with:

```go
// buildWranglerPassthrough returns the args to append after a `bun run <script>`
// (or configured command) invocation. They are placed behind a single `--`
// separator so they reach the underlying `wrangler dev` rather than bun/npm.
//
// Mode mapping:
//   local           -> --inspector-port <p> --local   (debugger on)
//   remote-bindings -> --inspector-port <p>            (debugger on; honors `remote: true` bindings)
//   remote           -> --remote                        (code on edge; no inspector, debugger off)
//
// `--env <env>` is appended for any mode when env is non-empty. Returns nil when
// there are no flags to add (so callers can append unconditionally).
func buildWranglerPassthrough(mode config.DevMode, inspectorPort int, env string) []string {
	if mode == "" {
		mode = config.DevModeRemoteBindings
	}

	var flags []string
	if mode != config.DevModeRemote && inspectorPort > 0 {
		flags = append(flags, "--inspector-port", strconv.Itoa(inspectorPort))
	}
	switch mode {
	case config.DevModeLocal:
		flags = append(flags, "--local")
	case config.DevModeRemote:
		flags = append(flags, "--remote")
	}
	if env != "" {
		flags = append(flags, "--env", env)
	}
	if len(flags) == 0 {
		return nil
	}
	return append([]string{"--"}, flags...)
}
```

(`strconv` and `config` are already imported in this file.)

- [x] **Step 4: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/process/ -run TestBuildWranglerPassthrough -v`
Expected: PASS. `go build ./...` will now fail at the old call sites — fixed in Task 4.

- [x] **Step 5: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/process/manager.go internal/process/passthrough_test.go
git commit -m "process: add buildWranglerPassthrough flag builder (replaces appendInspectorFlag)"
```

---

### Task 4: Wire the builder into `StartWorker` and `StartProcess`

**Files:**
- Modify: `internal/process/manager.go` (`StartWorker` ~line 147, `StartProcess` ~line 334)
- Modify: `internal/ui/table.go` (call sites in `startWorker` ~line 308 and `startAllWorkers` ~line 399)

- [x] **Step 1: Update `StartWorker`**

In `internal/process/manager.go`, in `StartWorker` replace the inspector-flag block (~lines 162-165):

```go
	runArgs := []string{"run", script}
	if w.InspectorPort > 0 {
		runArgs = appendInspectorFlag(runArgs, w.InspectorPort)
	}
```

with:

```go
	runArgs := []string{"run", script}
	runArgs = append(runArgs, buildWranglerPassthrough(w.DevMode, w.InspectorPort, w.SelectedEnv)...)
```

- [x] **Step 2: Update `StartProcess` signature to receive the worker**

In `internal/process/manager.go`, change the signature (~line 334):

```go
func (m *Manager) StartProcess(ctx context.Context, name string, cfg *config.Process, w *worker.Worker) (*Process, error) {
```

Replace the inspector-flag block (~lines 346-350):

```go
	args := cfg.Args
	if cfg.Type == config.ProcessTypeWorker && cfg.InspectorPort > 0 {
		args = appendInspectorFlag(args, cfg.InspectorPort)
	}
```

with:

```go
	args := cfg.Args
	if cfg.Type == config.ProcessTypeWorker {
		mode := cfg.DevMode
		inspectorPort := cfg.InspectorPort
		env := ""
		if w != nil {
			if w.DevMode != "" {
				mode = w.DevMode
			}
			env = w.SelectedEnv
			if w.InspectorPort > 0 {
				inspectorPort = w.InspectorPort
			}
		}
		args = append(args, buildWranglerPassthrough(mode, inspectorPort, env)...)
	}
```

Then, where the `process` struct is built (~line 389), set its worker so callers no longer need to patch it:

```go
	process := &Process{
		Name:      name,
		Worker:    w,
		Config:    cfg,
		Cmd:       cmd,
		Cancel:    cancel,
		LogBuffer: logBuffer,
		Started:   time.Now(),
	}
```

- [x] **Step 2b: Update the two `StartProcess` call sites in table.go**

In `internal/ui/table.go` `startWorker` (~line 307-313):

```go
				return m, func() tea.Msg {
					p, err := m.manager.StartProcess(context.Background(), workerName, &procCopy, w)
					_ = p
					return processStartedMsg{workerName: workerName, err: err}
				}
```

In `startAllWorkers` (~line 398-402):

```go
				if ws.isConfig {
					m.manager.StartProcess(context.Background(), ws.w.Name, ws.proc, ws.w)
				} else {
					m.manager.StartWorker(ws.w, ws.script)
				}
```

(`StartProcess` now sets `p.Worker` internally, so the old `p.Worker = w` patch lines are removed.)

- [x] **Step 3: Build and run the process test suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/process/ -v`
Expected: build succeeds (no remaining `appendInspectorFlag` references), tests PASS.

- [x] **Step 4: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/process/manager.go internal/ui/table.go
git commit -m "process: thread dev mode + selected env into StartWorker/StartProcess"
```

---

### Task 5: `ModeSelector` popup component

**Files:**
- Create: `internal/ui/mode_selector.go`

- [x] **Step 1: Create the component**

Create `internal/ui/mode_selector.go`:

```go
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/config"
	"wrkmn/internal/theme"
	"wrkmn/internal/worker"
)

type devModeOption struct {
	mode         config.DevMode
	label        string
	debuggable   bool
	touchesRemote bool
	desc         string
}

var devModeOptions = []devModeOption{
	{config.DevModeLocal, "local", true, false, "All bindings local. Debugger works."},
	{config.DevModeRemoteBindings, "remote-bindings", true, true, "Code local; honors remote: true bindings. Debugger works."},
	{config.DevModeRemote, "remote", false, true, "Code on Cloudflare's edge. No breakpoint debugging."},
}

// ModeSelector displays the available dev modes for a worker.
type ModeSelector struct {
	visible bool
	worker  *worker.Worker
	cursor  int
	width   int
	height  int
}

func NewModeSelector() *ModeSelector { return &ModeSelector{} }

func (s *ModeSelector) Show(w *worker.Worker) {
	if w == nil {
		return
	}
	s.visible = true
	s.worker = w
	s.cursor = 0
	for i, opt := range devModeOptions {
		if opt.mode == w.EffectiveDevMode() {
			s.cursor = i
			break
		}
	}
}

func (s *ModeSelector) Hide()          { s.visible = false }
func (s *ModeSelector) Visible() bool  { return s.visible }
func (s *ModeSelector) SetSize(w, h int) { s.width, s.height = w, h }

func (s *ModeSelector) Update(msg tea.Msg) tea.Cmd {
	if !s.visible {
		return nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "esc":
			s.Hide()
		case "j", "down":
			if s.cursor < len(devModeOptions)-1 {
				s.cursor++
			}
		case "k", "up":
			if s.cursor > 0 {
				s.cursor--
			}
		case "enter":
			s.worker.DevMode = devModeOptions[s.cursor].mode
			s.Hide()
		}
	}
	return nil
}

func (s *ModeSelector) View() string {
	if !s.visible || s.worker == nil {
		return ""
	}
	t := theme.Current
	overlayWidth := min(60, s.width-4)

	header := lipgloss.NewStyle().Foreground(t.Primary).Bold(true).
		Render(fmt.Sprintf("Select Dev Mode • %s", s.worker.Name))

	var lines []string
	selectedStyle := lipgloss.NewStyle().Background(t.Primary).Foreground(t.Background).Bold(true).Width(overlayWidth - 4)
	normalStyle := lipgloss.NewStyle().Foreground(t.Foreground).Width(overlayWidth - 4)
	descStyle := lipgloss.NewStyle().Foreground(t.Secondary)

	for i, opt := range devModeOptions {
		debug := "debug ✓"
		if !opt.debuggable {
			debug = "debug ✗"
		}
		remote := ""
		if opt.touchesRemote {
			remote = " • real resources"
		}
		marker := "  "
		if opt.mode == s.worker.EffectiveDevMode() {
			marker = "✓ "
		}
		title := fmt.Sprintf("%s%s  [%s%s]", marker, opt.label, debug, remote)
		if i == s.cursor {
			title = fmt.Sprintf("> %s  [%s%s]", opt.label, debug, remote)
			lines = append(lines, selectedStyle.Render(title))
		} else {
			lines = append(lines, normalStyle.Render(title))
		}
		lines = append(lines, descStyle.Render("    "+opt.desc))
	}

	footer := lipgloss.NewStyle().Foreground(t.Secondary).Render("Enter: select • j/k: navigate • Esc: cancel")

	body := lipgloss.JoinVertical(lipgloss.Left, header, "", strings.Join(lines, "\n"), "", footer)
	return lipgloss.NewStyle().Width(overlayWidth).Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).Padding(1, 2).Background(t.Background).Render(body)
}
```

- [x] **Step 2: Build to verify it compiles**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./...`
Expected: success. (`min` is already used by `env_selector.go`, so it resolves in this package.)

- [x] **Step 3: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/ui/mode_selector.go
git commit -m "ui: add ModeSelector popup for choosing a worker's dev mode"
```

---

### Task 6: Confirm dialog for full-remote starts

**Files:**
- Create: `internal/ui/confirm.go`

- [x] **Step 1: Create a minimal confirm-state component**

Create `internal/ui/confirm.go`:

```go
package ui

import (
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
)

// confirmDialog is a minimal y/n prompt. The owning model stores an onYes
// closure and runs it when the user confirms.
type confirmDialog struct {
	visible bool
	message string
	onYes   func(m TableModel) (TableModel, tea.Cmd)
}

func (c *confirmDialog) Show(message string, onYes func(m TableModel) (TableModel, tea.Cmd)) {
	c.visible = true
	c.message = message
	c.onYes = onYes
}

func (c *confirmDialog) Hide() {
	c.visible = false
	c.onYes = nil
}

func (c *confirmDialog) Visible() bool { return c.visible }

func (c *confirmDialog) View(width int) string {
	if !c.visible {
		return ""
	}
	t := theme.Current
	overlayWidth := min(60, width-4)
	header := lipgloss.NewStyle().Foreground(t.Warning).Bold(true).Render("Confirm full-remote start")
	body := lipgloss.NewStyle().Foreground(t.Foreground).Width(overlayWidth - 4).Render(c.message)
	footer := lipgloss.NewStyle().Foreground(t.Secondary).Render("y: start • n/Esc: cancel")
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(overlayWidth).Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Warning).Padding(1, 2).Background(t.Background).Render(content)
}
```

Add the bubbletea import to the file's import block (needed for the closure type):

```go
import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
)
```

- [x] **Step 2: Build to verify it compiles**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./...`
Expected: success.

- [x] **Step 3: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/ui/confirm.go
git commit -m "ui: add minimal confirm dialog for full-remote starts"
```

---

### Task 7: Table wiring — `m` key, selector, debug gate, confirm, footer

**Files:**
- Modify: `internal/ui/table.go`

- [x] **Step 1: Add fields to `TableModel` and constructor**

In `internal/ui/table.go` `TableModel` struct (~line 18), add:

```go
	modeSelector   *ModeSelector
	confirm        confirmDialog
```

In `NewTableModel` (~line 71), add to the struct literal:

```go
		modeSelector:   NewModeSelector(),
```

- [x] **Step 2: Intercept confirm + mode selector in `Update`**

In `Update`, in the `tea.KeyMsg` case (~line 157), add these checks **before** the `m.envSelector.Visible()` check:

```go
			// Confirm dialog takes priority
			if m.confirm.Visible() {
				switch msg.String() {
				case "y", "Y":
					onYes := m.confirm.onYes
					m.confirm.Hide()
					if onYes != nil {
						return onYes(m)
					}
					return m, nil
				case "n", "N", "esc", "q":
					m.confirm.Hide()
					return m, nil
				}
				return m, nil
			}

			// Mode selector
			if m.modeSelector.Visible() {
				cmd := m.modeSelector.Update(msg)
				return m, cmd
			}
```

- [x] **Step 3: Add the `m` keybinding and gate debug**

In `handleTableKeys` (~line 203), add a case alongside the others:

```go
		case key.Matches(msg, key.NewBinding(key.WithKeys("m"))):
			return m.showModeSelector()
```

Replace the existing `D` case (~line 223):

```go
		case key.Matches(msg, key.NewBinding(key.WithKeys("D"))):
			if m.cursor < len(m.workers) && !m.workers[m.cursor].SupportsDebugging() {
				m.statusMsg = "Debugging unavailable in full-remote mode (code runs on Cloudflare's edge)"
				return m, nil
			}
			return m.enterDebug()
```

- [x] **Step 4: Add `showModeSelector` and a full-remote guard for starts**

Add near `showEnvSelector` (~line 454):

```go
func (m TableModel) showModeSelector() (TableModel, tea.Cmd) {
	if m.cursor < len(m.workers) {
		m.modeSelector.Show(m.workers[m.cursor])
	}
	return m, nil
}
```

Wrap the single-worker start. Change the `d` case (~line 220) to route through a guard:

```go
		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			return m.startWorkerGuarded("dev")
```

Add the guard method near `startWorker`:

```go
// startWorkerGuarded confirms before starting a full-remote worker, otherwise
// starts immediately.
func (m TableModel) startWorkerGuarded(script string) (TableModel, tea.Cmd) {
	if m.cursor >= len(m.workers) {
		return m, nil
	}
	w := m.workers[m.cursor]
	if w.EffectiveDevMode() == config.DevModeRemote {
		m.confirm.Show(
			fmt.Sprintf("Start %q in FULL-REMOTE mode? Code runs on Cloudflare's edge and bindings hit real resources. Debugging is unavailable.", w.Name),
			func(mm TableModel) (TableModel, tea.Cmd) { return mm.startWorker(script) },
		)
		return m, nil
	}
	return m.startWorker(script)
}
```

- [x] **Step 5: Guard "start all" with a single batched confirm**

Change the `a` case (~line 232) to:

```go
		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			return m.startAllWorkersGuarded()
```

Add near `startAllWorkers`:

```go
// startAllWorkersGuarded shows one confirmation listing every worker that would
// start in full-remote mode, then starts all workers.
func (m TableModel) startAllWorkersGuarded() (TableModel, tea.Cmd) {
	var remote []string
	for _, w := range m.workers {
		if w.Status == worker.StatusRunning {
			continue
		}
		if w.EffectiveDevMode() == config.DevModeRemote {
			remote = append(remote, w.Name)
		}
	}
	if len(remote) > 0 {
		m.confirm.Show(
			fmt.Sprintf("These workers will start in FULL-REMOTE mode (code on edge, real resources, no debugging): %s. Continue?", strings.Join(remote, ", ")),
			func(mm TableModel) (TableModel, tea.Cmd) { return mm.startAllWorkers() },
		)
		return m, nil
	}
	return m.startAllWorkers()
}
```

(`fmt` and `strings` are already imported in table.go.)

- [x] **Step 6: Render the overlays**

In the `View` method, the env selector is overlaid via the package's own `overlayPopup` helper (NOT `lipgloss.Place`). Find the block (~line 482):

```go
	// Overlay env selector if visible
	if m.envSelector.Visible() {
		popupView := m.envSelector.View()
		return m.overlayPopup(tableView, popupView)
	}
```

and add, right after that `if` block closes:

```go
	// Overlay mode selector if visible
	if m.modeSelector.Visible() {
		popupView := m.modeSelector.View()
		return m.overlayPopup(tableView, popupView)
	}

	// Overlay full-remote confirm if visible
	if m.confirm.Visible() {
		popupView := m.confirm.View(m.width)
		return m.overlayPopup(tableView, popupView)
	}
```

- [x] **Step 7: Add `m: mode` to the footer hint**

In the footer render (~line 587), update the hint string to include mode:

```go
		Render("j/k: navigate • d: dev • D: debug • m: mode • s: scripts • r: remote logs • e: env • x: stop • a: all • l: logs • q: quit")
```

- [x] **Step 8: Build and run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./...`
Expected: build succeeds, all tests PASS.

- [x] **Step 9: Manual smoke test**

Run: `cd /home/jefflangston/dev/wrkmn && go build -o wrkmn . && ./wrkmn /home/jefflangston/dev/projects/stretch`
Verify: `m` opens the mode selector showing debug ✓/✗; selecting `remote` then `d` prompts a confirm; pressing `D` on a remote-mode worker shows the "Debugging unavailable" status. `Esc`/`n` cancels. Quit with `q`.

- [x] **Step 10: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/ui/table.go
git commit -m "ui: wire dev-mode selector (m), debug gate, and full-remote start confirm"
```

---

### Task 8: Document the modes + caveats

**Files:**
- Modify: `README.md` (Keybindings table; new "Dev modes" section)
- Modify: `CLAUDE.md` (Key Concepts → add dev modes)

- [x] **Step 1: Update README keybindings + add a Dev Modes section**

In `README.md` Main View keybindings table, add a row:

```markdown
| `m` | Select dev mode (local / remote-bindings / full-remote) |
```

After the "Process Types" section, add:

```markdown
## Dev Modes

Each worker runs in one of three modes (press `m` to switch; set a default with `dev_mode:` in `worker-compose.yaml`):

| Mode | `wrangler dev` | Bindings | Debugger |
| --- | --- | --- | --- |
| `local` | `--local` | all local simulations | ✅ |
| `remote-bindings` (default) | _(none)_ | honors each worker's `remote: true` bindings | ✅ |
| `remote` (full-remote) | `--remote` | all remote, code on edge | ❌ |

**Full-remote caveats:** slower iteration (upload per change), a 50-routes-per-zone
limit, deployed secrets are not loaded (use local `.dev.vars`), and breakpoint
debugging is unavailable. wrkmn asks for confirmation before starting a worker in
this mode.
```

- [x] **Step 2: Update CLAUDE.md Key Concepts**

In `CLAUDE.md` under "Process Types", add:

```markdown
**Dev Modes (workers):**
- `local` → `wrangler dev --local` (all bindings local; debugger on)
- `remote-bindings` (default) → plain `wrangler dev` (honors `remote: true` bindings; debugger on)
- `remote` → `wrangler dev --remote` (code on edge; debugger off)

Set per-process via `dev_mode:` in config, or switch at runtime with `m`.
```

- [x] **Step 3: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add README.md CLAUDE.md
git commit -m "docs: document worker dev modes and full-remote caveats"
```

---

## Self-review notes

- **Spec coverage:** mode→command mapping (Task 3); config default `remote-bindings` (Task 1); runtime `m` selector + config default (Tasks 5, 7); debug gating in full-remote (Task 7); full-remote confirm only (Tasks 6, 7); `--env` passthrough fix (Tasks 3, 4); caveats surfaced (Tasks 5, 8). No config-rewriting (correctly absent).
- **Type consistency:** `DevMode` constants (`DevModeLocal`/`DevModeRemoteBindings`/`DevModeRemote`) used identically across config, worker, process, and ui. `buildWranglerPassthrough(mode, inspectorPort, env)` signature consistent in definition (Task 3) and both call sites (Task 4). `StartProcess(ctx, name, cfg, w)` signature consistent across definition and both table.go call sites.
- **Placeholders:** none — every code step shows full code.
- **Risk:** Tasks 4, 6, 7 touch `table.go`/`manager.go`, which have in-flight uncommitted changes. Reconcile line numbers before editing. Overlay rendering uses the package's `overlayPopup(tableView, popupView)` helper (confirmed in the current tree), not `lipgloss.Place`.
```