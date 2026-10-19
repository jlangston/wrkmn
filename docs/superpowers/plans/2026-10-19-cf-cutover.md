# cf-only Cutover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace wrkmn's wrangler-shaped discovery/launch/debug pipeline with the cf CLI: `cloudflare.config.ts` discovery, `cf dev --mode/--local` launching, banner-driven port detection, and CDP debugger attach via the dev server's `/__debug` endpoint — TUI only.

**Architecture:** All cf-specific knowledge (config detection, Build Output, banner parsing, debug-ws extraction) lives in a new leaf package `internal/cfproject`. The process manager launches `cf dev` and feeds stdout through the banner parser to populate runtime `DevServerURL`/`DebugURL`/`Port`. The debugger discovers its websocket from `DebugURL` (fallback: `/json` scan on ports 9229–9249) and connects with the existing CDP engine unchanged except for breakpoint-hit classification. `internal/wrangler` is deleted; environments become modes.

**Tech Stack:** Go, Bubble Tea/Lipgloss, gorilla/websocket (existing), `go test`. External at runtime: `cf` CLI (v1 beta), verified by probes on 2026-10-01 and 2026-10-19.

**Spec:** `docs/superpowers/specs/2026-10-01-cf-cutover-design.md` (read it first — its Probe Findings section is the ground truth for banner shapes, ws URLs, and CDP behavior).

## Global Constraints

- Branch: `cf-cutover` (already exists, currently at the spec commits).
- Go standard library + existing deps only; no new modules.
- Every task ends with `go build ./... && go test ./...` green — no task leaves the tree broken for the next.
- Stage only the files each task lists; NEVER `git add -A`; never commit `.claude/settings.local.json`.
- **Commit messages carry NO AI-attribution trailers** (no `Co-Authored-By`, no "Generated with" lines) — user policy, overrides any harness default.
- gofmt-clean on every file touched (repo has ~22 pre-existing dirty files elsewhere — leave those alone).
- Probe-verified exact values (use verbatim): banner lines `  ➜  Local:   http://localhost:5173/` and `  ➜  Debug:   http://localhost:5173/__debug`; `/__debug` HTML embeds `ws=localhost%3A9229%2F<worker-name>`; `/json` returns `[{"webSocketDebuggerUrl":"ws://localhost:9229/<worker-name>",...}]`; concurrent instances increment to 5174/9230; breakpoint pauses arrive with `reason:"other"` and non-empty `hitBreakpoints`.

## Review Focus

Failure modes the spec implies but needs tests pinned to tasks (each line's test is included in the owning task below):

1. **ANSI-colored banner lines** — Vite colors its banner; `ParseBannerLine` must match through escape codes, or port/debug detection silently never fires (Task 1).
2. **Multi-worker `/__debug` HTML** — a dev session hosting several workers emits multiple `ws=` entries; `ExtractDebugWS` must select the requested worker, not blindly the first (Task 1).
3. **A dev server that never prints a Debug URL** (non-Vite fallback, broken project) — `D` must return a bounded, step-naming error, never hang the TUI (Task 5).
4. **Stale pre-cutover state file** (`selected_env` key, `dev_mode: "remote"`) — must migrate env→mode and drop `remote` without crashing or resurrecting removed modes (Task 3).
5. **A foreign process answering `/json` on a scanned port** — the fallback scan must match only a target whose ws path is exactly `/<worker-name>` (Task 5).

---

### Task 1: `internal/cfproject` package

**Files:**
- Create: `internal/cfproject/cfproject.go`
- Test: `internal/cfproject/cfproject_test.go`

**Interfaces:**
- Consumes: nothing (leaf package; stdlib only).
- Produces (exact signatures later tasks rely on):
  - `func Detect(dir string) (configPath string, ok bool)`
  - `func DetectUnmigrated(dir string) bool`
  - `type BuildOutput struct { WorkerName string; AccountID string }`
  - `func ReadBuildOutput(dir string) (*BuildOutput, error)` — missing file ⇒ `(nil, nil)`
  - `type BannerKind int` with `BannerLocal`, `BannerDebug`
  - `func ParseBannerLine(line string) (kind BannerKind, url string, ok bool)`
  - `func ExtractDebugWS(ctx context.Context, debugURL, workerName string) (wsURL string, err error)`

- [ ] **Step 1: Write the failing tests**

Create `internal/cfproject/cfproject_test.go`:

```go
package cfproject

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Detect(dir); ok {
		t.Fatal("empty dir detected as cf project")
	}
	writeFile(t, dir, "cloudflare.config.ts", "export default {}")
	path, ok := Detect(dir)
	if !ok || path != filepath.Join(dir, "cloudflare.config.ts") {
		t.Fatalf("Detect = (%q, %v)", path, ok)
	}
}

func TestDetectVariants(t *testing.T) {
	for _, name := range []string{"cloudflare.config.js", "cloudflare.config.mjs", "cloudflare.config.mts"} {
		dir := t.TempDir()
		writeFile(t, dir, name, "export default {}")
		if _, ok := Detect(dir); !ok {
			t.Fatalf("%s not detected", name)
		}
	}
}

func TestDetectUnmigrated(t *testing.T) {
	dir := t.TempDir()
	if DetectUnmigrated(dir) {
		t.Fatal("empty dir reported unmigrated")
	}
	writeFile(t, dir, "wrangler.toml", "name = \"x\"")
	if !DetectUnmigrated(dir) {
		t.Fatal("wrangler.toml-only dir not reported unmigrated")
	}
	writeFile(t, dir, "cloudflare.config.ts", "export default {}")
	if DetectUnmigrated(dir) {
		t.Fatal("migrated dir still reported unmigrated")
	}
}

func TestReadBuildOutput(t *testing.T) {
	dir := t.TempDir()
	bo, err := ReadBuildOutput(dir)
	if bo != nil || err != nil {
		t.Fatalf("missing build output should be (nil, nil), got (%v, %v)", bo, err)
	}
	outDir := filepath.Join(dir, ".cloudflare", "output", "v0")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, outDir, "config.json", `{"name":"api","account_id":"abc123","unknown_field":[1,2]}`)
	bo, err = ReadBuildOutput(dir)
	if err != nil || bo == nil || bo.WorkerName != "api" || bo.AccountID != "abc123" {
		t.Fatalf("ReadBuildOutput = (%+v, %v)", bo, err)
	}
}

func TestParseBannerLine(t *testing.T) {
	cases := []struct {
		line string
		kind BannerKind
		url  string
		ok   bool
	}{
		{"  ➜  Local:   http://localhost:5173/", BannerLocal, "http://localhost:5173/", true},
		{"  ➜  Debug:   http://localhost:5173/__debug", BannerDebug, "http://localhost:5173/__debug", true},
		// Review Focus 1: ANSI-colored banner (green arrow, bold label, cyan URL)
		{"\x1b[32m  ➜\x1b[39m  \x1b[1mLocal\x1b[22m:   \x1b[36mhttp://localhost:5174/\x1b[39m", BannerLocal, "http://localhost:5174/", true},
		{"\x1b[32m  ➜\x1b[39m  Debug:   \x1b[36mhttp://localhost:5174/__debug\x1b[39m", BannerDebug, "http://localhost:5174/__debug", true},
		{"  ➜  Network: use --host to expose", 0, "", false},
		{"VITE v8.3.2 ready in 3433 ms", 0, "", false},
		{"", 0, "", false},
	}
	for _, c := range cases {
		kind, url, ok := ParseBannerLine(c.line)
		if kind != c.kind || url != c.url || ok != c.ok {
			t.Fatalf("ParseBannerLine(%q) = (%v, %q, %v), want (%v, %q, %v)", c.line, kind, url, ok, c.kind, c.url, c.ok)
		}
	}
}

// debugHTML mirrors the real /__debug payload captured in the 2026-10-01 probe.
func debugHTML(entries ...string) string {
	list := ""
	for i, e := range entries {
		if i > 0 {
			list += ","
		}
		list += `"https://devtools.devprod.cloudflare.dev/js_app?theme=systemPreferred&debugger=true&ws=` + e + `&domain=x"`
	}
	return `<script>const workerUrls = [` + list + `];</script>`
}

func TestExtractDebugWS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(debugHTML("localhost%3A9229%2Fcf-probe")))
	}))
	defer srv.Close()
	ws, err := ExtractDebugWS(context.Background(), srv.URL, "cf-probe")
	if err != nil || ws != "ws://localhost:9229/cf-probe" {
		t.Fatalf("ExtractDebugWS = (%q, %v)", ws, err)
	}
}

func TestExtractDebugWSMultiWorker(t *testing.T) {
	// Review Focus 2: multiple workers in one dev session — pick by name.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(debugHTML("localhost%3A9229%2Fstretch-api", "localhost%3A9229%2Fstretch-render")))
	}))
	defer srv.Close()
	ws, err := ExtractDebugWS(context.Background(), srv.URL, "stretch-render")
	if err != nil || ws != "ws://localhost:9229/stretch-render" {
		t.Fatalf("multi-worker pick = (%q, %v)", ws, err)
	}
	if _, err := ExtractDebugWS(context.Background(), srv.URL, "no-such-worker"); err == nil {
		t.Fatal("expected error when no entry matches the worker name")
	}
}

func TestExtractDebugWSNoEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>nothing here</html>"))
	}))
	defer srv.Close()
	if _, err := ExtractDebugWS(context.Background(), srv.URL, "x"); err == nil {
		t.Fatal("expected error for page without ws entries")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/cfproject/ -v`
Expected: compile error (package does not exist).

- [ ] **Step 3: Implement the package**

Create `internal/cfproject/cfproject.go`:

```go
// Package cfproject holds all cf-CLI-specific knowledge: project detection,
// Build Output reading, dev-server banner parsing, and debug-websocket
// extraction. The cf CLI is in beta; when its surfaces drift, this package is
// the single place to fix. Shapes verified against cf 1.0.0-beta.10 and
// @cloudflare/vite-plugin 2.0.0-beta (see the cutover spec's probe findings).
package cfproject

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// configNames are the cf project config files, in detection order.
var configNames = []string{
	"cloudflare.config.ts",
	"cloudflare.config.mts",
	"cloudflare.config.js",
	"cloudflare.config.mjs",
}

// wranglerNames mark an unmigrated Wrangler project.
var wranglerNames = []string{"wrangler.toml", "wrangler.jsonc", "wrangler.json"}

// Detect reports whether dir is a cf project and returns the config path.
func Detect(dir string) (string, bool) {
	for _, name := range configNames {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// DetectUnmigrated reports a directory that has a Wrangler config but no cf
// config — a project that needs `cf migrate` before wrkmn can manage it.
func DetectUnmigrated(dir string) bool {
	if _, ok := Detect(dir); ok {
		return false
	}
	for _, name := range wranglerNames {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// BuildOutput is the subset of .cloudflare/output/v0/config.json wrkmn reads.
type BuildOutput struct {
	WorkerName string
	AccountID  string
}

// ReadBuildOutput loads Build Output if present. A missing file returns
// (nil, nil): enrichment is strictly opportunistic. The format is beta, so
// parsing is lenient — unknown fields ignored, several key spellings probed.
func ReadBuildOutput(dir string) (*BuildOutput, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".cloudflare", "output", "v0", "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("build output config.json: %w", err)
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	return &BuildOutput{
		WorkerName: str("name", "worker_name", "workerName"),
		AccountID:  str("account_id", "accountId"),
	}, nil
}

// BannerKind identifies a recognized dev-server banner line.
type BannerKind int

const (
	BannerLocal BannerKind = iota + 1 // "Local:" — the serving URL
	BannerDebug                       // "Debug:" — the /__debug URL
)

var (
	ansiRe   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	localRe  = regexp.MustCompile(`Local:\s+(https?://\S+)`)
	debugRe  = regexp.MustCompile(`Debug:\s+(https?://\S+)`)
	wsRe     = regexp.MustCompile(`[?&]ws=([^&"'\]\s]+)`)
)

// ParseBannerLine matches the Vite/cf dev startup banner's Local and Debug
// lines. ANSI color codes are stripped before matching.
func ParseBannerLine(line string) (BannerKind, string, bool) {
	plain := ansiRe.ReplaceAllString(line, "")
	if m := debugRe.FindStringSubmatch(plain); m != nil {
		return BannerDebug, m[1], true
	}
	if m := localRe.FindStringSubmatch(plain); m != nil {
		return BannerLocal, m[1], true
	}
	return 0, "", false
}

// ExtractDebugWS fetches the dev server's /__debug page and extracts the CDP
// websocket for workerName. The page embeds one hosted-DevTools URL per
// worker, each carrying a URL-encoded "ws=host:port/worker-name" parameter.
// With a single entry, the name need not match (single-worker session); with
// several, the entry whose path is "/<workerName>" is required.
func ExtractDebugWS(ctx context.Context, debugURL, workerName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, debugURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", debugURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var endpoints []string
	for _, m := range wsRe.FindAllStringSubmatch(string(body), -1) {
		decoded, err := url.QueryUnescape(m[1])
		if err != nil {
			continue
		}
		endpoints = append(endpoints, decoded)
	}
	if len(endpoints) == 0 {
		return "", fmt.Errorf("%s contains no ws= debug endpoints", debugURL)
	}
	if len(endpoints) == 1 {
		return "ws://" + endpoints[0], nil
	}
	for _, ep := range endpoints {
		if strings.HasSuffix(ep, "/"+workerName) {
			return "ws://" + ep, nil
		}
	}
	return "", fmt.Errorf("%s lists %d debug endpoints but none ends in /%s", debugURL, len(endpoints), workerName)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/cfproject/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/cfproject/
git commit -m "cfproject: add cf project detection, banner parsing, and debug-ws extraction"
```

---

### Task 2: Remove the full-remote dev mode

**Files:**
- Modify: `internal/config/types.go` (DevMode consts), `internal/config/loader.go` (validate), `internal/worker/types.go` (`SupportsDebugging`), `internal/state/state.go` (`Apply` allowlist), `internal/ui/mode_selector.go` (options), `internal/ui/table.go` (guards), `internal/process/manager.go` (`buildWranglerPassthrough` switch)
- Test: `internal/config/loader_test.go`, `internal/state/state_test.go`, `internal/process/passthrough_test.go`, `internal/config/types_test.go` (update existing expectations)

**Interfaces:**
- Consumes: existing DevMode machinery.
- Produces: `config.DevMode` has exactly two values (`DevModeLocal`, `DevModeRemoteBindings`); `dev_mode: remote` is a validation error whose message contains `full-remote dev does not exist in the cf CLI`; `(*worker.Worker).SupportsDebugging()` returns true unconditionally; `startWorkerGuarded`/`startAllWorkersGuarded` are gone (keys call `startWorker`/`startAllWorkers` directly).

- [ ] **Step 1: Write/adjust the failing tests**

In `internal/config/loader_test.go`, add:

```go
func TestDevModeRemoteRejectedWithHint(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: cf
    args: [dev]
    type: worker
    dev_mode: remote
`)
	_, err := NewLoader(path).Load()
	if err == nil {
		t.Fatal("dev_mode: remote should be rejected")
	}
	if !strings.Contains(err.Error(), "full-remote dev does not exist in the cf CLI") {
		t.Fatalf("error lacks migration hint: %v", err)
	}
}
```

(Add `"strings"` to the test file imports if absent.)

In `internal/state/state_test.go`, update `TestApplyOverridesDefaultsAndIgnoresBogusModes` so the ignored value is `"remote"` (previously a valid mode, now must be dropped):

```go
	Apply(map[string]WorkerState{
		"api": {DevMode: "local", SelectedEnv: "staging"},
		"web": {DevMode: "remote"}, // removed mode — must be ignored
	}, workers)
```

(keep the rest of that test's assertions; `web.DevMode` must stay `""`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run TestDevModeRemoteRejected -v && go test ./internal/state/ -v`
Expected: the config test FAILs (remote currently accepted); the state test FAILs (remote currently applied).

- [ ] **Step 3: Remove DevModeRemote from config**

In `internal/config/types.go`, delete the `DevModeRemote` constant and reword the block:

```go
// DevMode selects how a worker's `cf dev` connects to resources.
type DevMode string

const (
	// DevModeLocal forces all bindings to local simulations (cf dev --local).
	DevModeLocal DevMode = "local"
	// DevModeRemoteBindings runs code locally and honors the worker's own
	// remote bindings (plain cf dev). This is the default.
	DevModeRemoteBindings DevMode = "remote-bindings"
)
```

In `internal/config/loader.go` `validate`, the dev_mode switch becomes:

```go
	// Validate dev_mode
	for name, proc := range cfg.Processes {
		switch proc.DevMode {
		case "", DevModeLocal, DevModeRemoteBindings:
			// Valid (empty defaults later)
		default:
			return fmt.Errorf("process %q: invalid dev_mode %q (must be local or remote-bindings; full-remote dev does not exist in the cf CLI)", name, proc.DevMode)
		}
	}
```

- [ ] **Step 4: Update the consumers**

- `internal/worker/types.go`: `SupportsDebugging` becomes

```go
// SupportsDebugging reports whether breakpoint debugging is available. Both
// cf dev modes run code locally, so this is always true.
func (w *Worker) SupportsDebugging() bool {
	return true
}
```

- `internal/state/state.go` `Apply`: drop `config.DevModeRemote` from the case list.
- `internal/ui/mode_selector.go`: reduce `devModeOptions` to two entries and drop the `debuggable`/`touchesRemote` struct fields and their rendering (the `[debug ✓ …]` suffix); descriptions become `"All bindings local (cf dev --local)."` and `"Code local; remote bindings honored (plain cf dev). Default."`.
- `internal/ui/table.go`: the `d` key case calls `m.startWorker("dev")` directly and the `a` key case calls `m.startAllWorkersGuarded` → `m.startAllWorkers()`; delete `startWorkerGuarded` and `startAllWorkersGuarded` entirely. Keep `confirm.go` and the confirm overlay/Update handling (generic, now dormant). Delete the `D`-key `SupportsDebugging` gate (the check is now constant-true; remove the `if` so the status message can't appear).
- `internal/process/manager.go` `buildWranglerPassthrough`: remove the `case config.DevModeRemote` branch and the `mode != config.DevModeRemote` qualifier on the inspector-port append (becomes `if inspectorPort > 0`); update its doc comment's mode table to the two modes. In `internal/process/passthrough_test.go`, delete the two `remote` rows.
- `internal/config/types_test.go` / any other test referencing `DevModeRemote`: grep (`grep -rn DevModeRemote --include='*_test.go'`) and update each to the new expectations (the renumbering note: there is no renumbering — these are strings).

- [ ] **Step 5: Run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./...`
Expected: PASS everywhere.

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/config/ internal/worker/types.go internal/state/ internal/ui/mode_selector.go internal/ui/table.go internal/process/manager.go internal/process/passthrough_test.go
git commit -m "config: remove full-remote dev mode (no cf equivalent)"
```

(If `internal/config/types_test.go` or other test files changed, stage them too — only files actually touched.)

---

### Task 3: Modes replace environments

**Files:**
- Modify: `internal/config/types.go` (Process gets `Modes`, `AccountID`), `internal/config/loader.go` (validation + defaults), `internal/worker/types.go` (field renames + new fields), `internal/worker/discoverer.go` (renamed field refs only), `internal/ui/env_selector.go`, `internal/ui/remote_logs.go`, `internal/ui/table.go` (selector gate), `internal/state/state.go`, `cmd/root.go` (seeding)
- Test: `internal/config/loader_test.go`, `internal/state/state_test.go`

**Interfaces:**
- Consumes: Task 2's two-mode DevMode.
- Produces: `config.Process.Modes []string` (`yaml:"modes,omitempty"`) and `config.Process.AccountID string` (`yaml:"account_id,omitempty"`); worker fields `Modes []string`, `SelectedMode string` (json `selected_mode`), `DevServerURL string`, `DebugURL string`, `HasModes() bool`; `state.WorkerState.SelectedMode` (json `selected_mode`) with legacy `selected_env` read; default worker modes `["development", "production"]`.

- [ ] **Step 1: Write the failing tests**

In `internal/config/loader_test.go`:

```go
func TestModesDefaultForWorkers(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: cf
    args: [dev]
    type: worker
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := cfg.Processes["api"].Modes
	if len(got) != 2 || got[0] != "development" || got[1] != "production" {
		t.Fatalf("Modes = %v, want default [development production]", got)
	}
}

func TestModesExplicitAndValidated(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: cf
    args: [dev]
    type: worker
    modes: [development, staging, production]
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Processes["api"].Modes) != 3 {
		t.Fatalf("Modes = %v", cfg.Processes["api"].Modes)
	}

	bad := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: cf
    type: worker
    modes: [development, development]
`)
	if _, err := NewLoader(bad).Load(); err == nil {
		t.Fatal("duplicate modes should be rejected")
	}
	empty := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: cf
    type: worker
    modes: ["", staging]
`)
	if _, err := NewLoader(empty).Load(); err == nil {
		t.Fatal("empty mode string should be rejected")
	}
}
```

In `internal/state/state_test.go` (Review Focus 4 — replace the round-trip test's field names and add the legacy test):

```go
func TestLoadMigratesLegacySelectedEnv(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"workers":{"api":{"dev_mode":"remote","selected_env":"staging"}}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	states, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if states["api"].SelectedMode != "staging" {
		t.Fatalf("legacy selected_env not migrated: %+v", states["api"])
	}
	// dev_mode "remote" survives Load but must be dropped by Apply (Task 2 allowlist).
	workers := []*worker.Worker{{Name: "api"}}
	Apply(states, workers)
	if workers[0].DevMode != "" || workers[0].SelectedMode != "staging" {
		t.Fatalf("Apply = DevMode %q SelectedMode %q", workers[0].DevMode, workers[0].SelectedMode)
	}
}
```

Also rename `SelectedEnv` → `SelectedMode` in the existing state tests (`TestSaveLoadRoundTrip`, `TestApplyOverridesDefaultsAndIgnoresBogusModes`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run TestModes -v; go test ./internal/state/`
Expected: compile errors / FAILs (`Modes` and `SelectedMode` undefined).

- [ ] **Step 3: Config fields, validation, defaults**

`internal/config/types.go` — in the `Process` struct, next to `DevMode`:

```go
	// Modes a worker's cloudflare.config.ts function accepts (cf dev --mode).
	// Defaults to [development, production] for worker-type processes.
	Modes []string `yaml:"modes,omitempty"`
	// AccountID for remote operations (log tailing). Optional; falls back to
	// Build Output and then the CLOUDFLARE_ACCOUNT_ID environment variable.
	AccountID string `yaml:"account_id,omitempty"`
```

`internal/config/loader.go` — in `validate`, after the dev_mode block:

```go
	// Validate modes
	for name, proc := range cfg.Processes {
		seen := make(map[string]bool)
		for _, m := range proc.Modes {
			if m == "" {
				return fmt.Errorf("process %q: modes entries must be non-empty strings", name)
			}
			if seen[m] {
				return fmt.Errorf("process %q: duplicate mode %q", name, m)
			}
			seen[m] = true
		}
	}
```

In `applyDefaults`, after the dev_mode default:

```go
		// Default modes for workers (cf dev --mode values the selector offers)
		if proc.Type == ProcessTypeWorker && len(proc.Modes) == 0 {
			proc.Modes = []string{"development", "production"}
		}
```

- [ ] **Step 4: Worker fields and consumers**

`internal/worker/types.go`:
- Rename `Environments []string` → `Modes []string` (json `modes,omitempty`), `SelectedEnv` → `SelectedMode` (json `selected_mode,omitempty`), with comments mentioning cf modes.
- Add after them:

```go
	// DevServerURL and DebugURL are discovered at runtime from the cf dev
	// startup banner (see internal/cfproject.ParseBannerLine). Written by the
	// manager's log reader; same unsynchronized-field pattern as Status
	// (parked architectural debt).
	DevServerURL string `json:"dev_server_url,omitempty"`
	DebugURL     string `json:"debug_url,omitempty"`
```

- `HasEnvironments()` → `HasModes() bool { return len(w.Modes) > 0 }`.
- `GetCurrentAccountID()`/`GetAccountIDForEnv`: change `GetCurrentAccountID` to `return w.AccountID` directly (drop the WranglerConfig lookup); `GetAccountIDForEnv` is deleted (grep confirms its only caller is `GetCurrentAccountID`).

`internal/worker/discoverer.go` (`EnrichFromWranglerConfig`): rename the field writes — `w.Modes = config.GetEnvironments()`, delete the `w.SelectedEnv = "production"` line (SelectedMode's zero value means "cf default"). This function is deleted wholesale in Task 6; this step only keeps the build green.

`internal/ui/env_selector.go`: replace `Environments` → `Modes`, `SelectedEnv` → `SelectedMode` throughout; retitle the popup `"Select Mode • %s"`; update the file's doc comments from environments to modes (the type stays `EnvSelector` to keep this task mechanical — Task 9's docs call it the mode selector).

`internal/ui/remote_logs.go`: `r.worker.SelectedEnv` → `r.worker.SelectedMode` (both references; the `NewTailSession(client, accountID, name, environment)` signature is unchanged — the mode string is passed as the environment argument, and the tail session's existing `!= "production"` handling still applies).

`internal/ui/table.go` `showEnvSelector`: `w.HasEnvironments()` → `w.HasModes()`.

`internal/state/state.go`:

```go
type WorkerState struct {
	DevMode      string `json:"dev_mode,omitempty"`
	SelectedMode string `json:"selected_mode,omitempty"`
	// LegacySelectedEnv migrates pre-cutover state files; read-only.
	LegacySelectedEnv string `json:"selected_env,omitempty"`
}
```

In `Load`, after unmarshal, migrate:

```go
	for name, st := range f.Workers {
		if st.SelectedMode == "" && st.LegacySelectedEnv != "" {
			st.SelectedMode = st.LegacySelectedEnv
		}
		st.LegacySelectedEnv = ""
		f.Workers[name] = st
	}
```

`Save`: write `SelectedMode` from `w.SelectedMode` (skip-empties condition updates to `w.DevMode == "" && w.SelectedMode == ""`). `Apply`: set `w.SelectedMode = st.SelectedMode` when non-empty.

`cmd/root.go`: in the config-worker seeding loop add `Modes: proc.Modes,` and `AccountID: proc.AccountID,` to the `worker.Worker` literal.

- [ ] **Step 5: Run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./...`
Expected: PASS (update any straggler test referencing the old field names — find them with `grep -rn "SelectedEnv\|Environments\b" --include='*_test.go' internal/`).

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/config/ internal/worker/ internal/ui/env_selector.go internal/ui/remote_logs.go internal/ui/table.go internal/state/ cmd/root.go
git commit -m "worker: environments become cf modes; selected_mode persisted with legacy migration"
```

---

### Task 4: Launch cutover (`cf dev` + banner-driven ports)

**Files:**
- Modify: `internal/process/manager.go`
- Modify: `internal/ui/table.go` (StartWorker call sites)
- Test: rename `internal/process/passthrough_test.go` → `internal/process/cfargs_test.go`; append a banner-wiring test to `internal/process/manager_test.go`

**Interfaces:**
- Consumes: Task 1's `cfproject.ParseBannerLine`/`BannerLocal`/`BannerDebug`; Task 3's `SelectedMode`/`DevServerURL`/`DebugURL`.
- Produces: `buildCfDevArgs(devMode config.DevMode, mode string) []string`; `(m *Manager) StartWorker(w *worker.Worker) error` (script parameter removed — discovered workers always run `cf dev`); `readLogs(reader, buffer, prefix string, w *worker.Worker)` populating `DevServerURL`/`DebugURL`/`Port`. `buildWranglerPassthrough` and `Process.Script` are gone.

- [ ] **Step 1: Write the failing tests**

Create `internal/process/cfargs_test.go` (delete `passthrough_test.go` in the same change):

```go
package process

import (
	"reflect"
	"testing"

	"wrkmn/internal/config"
)

func TestBuildCfDevArgs(t *testing.T) {
	cases := []struct {
		name string
		dev  config.DevMode
		mode string
		want []string
	}{
		{"defaults yield nothing", config.DevModeRemoteBindings, "", nil},
		{"empty dev mode treated as remote-bindings", "", "", nil},
		{"local", config.DevModeLocal, "", []string{"--local"}},
		{"mode only", config.DevModeRemoteBindings, "staging", []string{"--mode", "staging"}},
		{"local with mode", config.DevModeLocal, "staging", []string{"--local", "--mode", "staging"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildCfDevArgs(c.dev, c.mode); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
```

Append to `internal/process/manager_test.go`:

```go
func TestBannerParsingPopulatesWorkerURLs(t *testing.T) {
	m := NewManager()
	w := &worker.Worker{Name: "bannered", Status: worker.StatusStarting}
	cfg := &config.Process{
		Command: "sh",
		Args: []string{"-c",
			`printf '  \xe2\x9e\x9c  Local:   http://localhost:8123/\n  \xe2\x9e\x9c  Debug:   http://localhost:8123/__debug\n'; sleep 30`},
		Type: config.ProcessTypeWorker,
	}
	if _, err := m.StartProcess(context.Background(), "bannered", cfg, w); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer m.StopAll()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if w.DebugURL != "" && w.Port == 8123 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("banner not parsed: Port=%d DevServerURL=%q DebugURL=%q", w.Port, w.DevServerURL, w.DebugURL)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/process/ -run "TestBuildCfDevArgs|TestBannerParsing" -v`
Expected: compile error (`buildCfDevArgs` undefined).

- [ ] **Step 3: Implement the args builder and banner hook**

In `internal/process/manager.go`, replace `buildWranglerPassthrough` with (import `wrkmn/internal/cfproject` and `net/url`):

```go
// buildCfDevArgs returns the arguments appended to a worker launch command.
// There is no "--" separator: `cf dev` takes these flags directly, and
// `bun run <script>` forwards trailing args to the script it runs.
//
//	local           -> --local        (all bindings simulated locally)
//	remote-bindings -> (nothing; cf dev's default)
//	mode != ""      -> --mode <mode>  (selects the cloudflare.config.ts mode)
func buildCfDevArgs(devMode config.DevMode, mode string) []string {
	var args []string
	if devMode == config.DevModeLocal {
		args = append(args, "--local")
	}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	return args
}

// applyBannerLine feeds one raw stdout line through the cf dev banner parser
// and records discovered URLs on the worker. Writes follow the existing
// unsynchronized Worker-field pattern (parked debt; see Status).
func applyBannerLine(w *worker.Worker, line string) {
	if w == nil {
		return
	}
	kind, u, ok := cfproject.ParseBannerLine(line)
	if !ok {
		return
	}
	switch kind {
	case cfproject.BannerLocal:
		w.DevServerURL = u
		if parsed, err := url.Parse(u); err == nil {
			if p := parsed.Port(); p != "" {
				if n, err := strconv.Atoi(p); err == nil {
					w.Port = n
				}
			}
		}
	case cfproject.BannerDebug:
		w.DebugURL = u
	}
}
```

Change `readLogs` to accept the worker and parse raw lines before formatting:

```go
func (m *Manager) readLogs(reader io.Reader, buffer *CircularBuffer, prefix string, w *worker.Worker) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		raw := scanner.Text()
		applyBannerLine(w, raw)
		line := fmt.Sprintf("[%s] %s: %s", time.Now().Format("15:04:05"), prefix, raw)
		buffer.Write(line)
	}
}
```

Update all four `go m.readLogs(...)` call sites (stdout/stderr in `StartWorker` and `StartProcess`) to pass the process's worker (`w`, which may be nil in `StartProcess` — `applyBannerLine` guards).

- [ ] **Step 4: Rework StartWorker and the worker branch of StartProcess**

`StartWorker` becomes script-less — discovered cf workers always run `cf dev`:

```go
func (m *Manager) StartWorker(w *worker.Worker) error {
```

Inside, replace the script lookup and `bun` command construction with:

```go
	runArgs := append([]string{"dev"}, buildCfDevArgs(w.DevMode, w.SelectedMode)...)
	cmd := exec.CommandContext(ctx, "cf", runArgs...)
	cmd.Dir = w.Path
```

Delete the `_, exists := w.Scripts[script]` guard and the `Script:` field from the `Process` literal; delete the `Script` field from the `Process` struct (its only writer was here).

In `StartProcess`'s worker branch, replace the passthrough block with:

```go
	args := append([]string{}, cfg.Args...)
	if cfg.Type == config.ProcessTypeWorker {
		mode := cfg.DevMode
		selectedMode := ""
		if w != nil {
			if w.DevMode != "" {
				mode = w.DevMode
			}
			selectedMode = w.SelectedMode
		}
		args = append(args, buildCfDevArgs(mode, selectedMode)...)
	}
```

(The `InspectorPort` plumbing in this block is deleted; the config field itself survives until Task 7's deprecation warning.)

In `internal/ui/table.go`: `startWorker`'s discovered branch becomes `err := m.manager.StartWorker(w)` (drop the `script` argument and the `"dev"`/`"start"` fallback selection); in `startAllWorkers`, the `discoveredStart` struct loses its `script` field, the script-fallback block goes away (a discovered cf worker is always startable), and the loop calls `manager.StartWorker(ds.w)`. The `startWorker(script string)` parameter itself can be dropped along with the `d`-key call site passing `"dev"` — the parameter is now meaningless.

- [ ] **Step 5: Run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./... && go test -race ./internal/process/ ./internal/ui/`
Expected: PASS. Existing manager/scheduler/ui tests that constructed workers with Scripts maps still pass (Scripts is only informational now for discovered workers; the `_config` marker path is untouched).

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/process/ internal/ui/table.go
git commit -m "process: launch workers via cf dev; parse dev-server banner for ports and debug URL"
```

---

### Task 5: Debugger attach via the debug endpoint

**Files:**
- Create: `internal/debug/discover.go`
- Modify: `internal/debug/session.go` (`handlePaused`), `internal/ui/table_debug.go` (`enterDebug`)
- Test: `internal/debug/discover_test.go` (create), `internal/debug/session_test.go` (create or append)

**Interfaces:**
- Consumes: Task 1's `cfproject.ExtractDebugWS`; Task 3's `Worker.DebugURL`; existing `Session.Connect(*InspectorTarget)`, `InspectorDiscovery`.
- Produces: `var DefaultFallbackPorts []int` (9229–9249); `func DiscoverWorkerWS(ctx context.Context, workerName string, debugURL func() string, fallbackPorts []int, timeout time.Duration) (string, error)`; `Debugger.paused` events with non-empty `hitBreakpoints` surface as `PauseReasonBreakpoint`.

- [ ] **Step 1: Write the failing tests**

Create `internal/debug/discover_test.go`:

```go
package debug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDiscoverWorkerWSPrimary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<script>const workerUrls = ["https://devtools.devprod.cloudflare.dev/js_app?debugger=true&ws=localhost%3A9229%2Fapi&domain=api"];</script>`))
	}))
	defer srv.Close()
	debugURL := func() string { return srv.URL }
	ws, err := DiscoverWorkerWS(context.Background(), "api", debugURL, nil, 3*time.Second)
	if err != nil || ws != "ws://localhost:9229/api" {
		t.Fatalf("DiscoverWorkerWS = (%q, %v)", ws, err)
	}
}

func TestDiscoverWorkerWSWaitsForBanner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<script>const workerUrls = ["https://x/js_app?ws=localhost%3A9229%2Fapi"];</script>`))
	}))
	defer srv.Close()
	var url string
	go func() { time.Sleep(400 * time.Millisecond); url = srv.URL }()
	ws, err := DiscoverWorkerWS(context.Background(), "api", func() string { return url }, nil, 3*time.Second)
	if err != nil || ws != "ws://localhost:9229/api" {
		t.Fatalf("late banner not handled: (%q, %v)", ws, err)
	}
}

func TestDiscoverWorkerWSFallbackScan(t *testing.T) {
	// Review Focus 5: a /json listing with a foreign target plus the real one —
	// match strictly by "/<worker>" path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `[
			{"webSocketDebuggerUrl":"ws://localhost:9229/some-node-process","title":"node"},
			{"webSocketDebuggerUrl":"ws://localhost:9229/api","title":"Cloudflare Worker"}
		]`)
	}))
	defer srv.Close()
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	ws, err := DiscoverWorkerWS(context.Background(), "api", func() string { return "" }, []int{port}, 3*time.Second)
	if err != nil || ws != "ws://localhost:9229/api" {
		t.Fatalf("fallback = (%q, %v)", ws, err)
	}
	// No match for an unknown worker — must error, not grab the first target.
	if _, err := DiscoverWorkerWS(context.Background(), "ghost", func() string { return "" }, []int{port}, 1*time.Second); err == nil {
		t.Fatal("expected no-match error for unknown worker")
	}
}

func TestDiscoverWorkerWSTimeoutNamesSteps(t *testing.T) {
	// Review Focus 3: nothing ever appears — bounded error naming what failed.
	start := time.Now()
	_, err := DiscoverWorkerWS(context.Background(), "api", func() string { return "" }, nil, 700*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout not bounded")
	}
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "Debug URL") {
		t.Fatalf("error not descriptive: %v", err)
	}
}
```

Create/append `internal/debug/session_test.go`:

```go
package debug

import "testing"

func TestHandlePausedHitBreakpointsOverridesReason(t *testing.T) {
	s := NewSession()
	// The Vite-plugin inspector reports reason "other" on breakpoint hits
	// (verified 2026-10-19); hitBreakpoints is the authoritative signal.
	s.handlePaused(map[string]any{
		"reason":         "other",
		"hitBreakpoints": []any{"1:4:0:/src/index.ts"},
		"callFrames":     []any{},
	})
	ev := <-s.Events()
	pe, ok := ev.Data.(PausedEvent)
	if !ok {
		t.Fatalf("event data %T", ev.Data)
	}
	if pe.Reason != PauseReasonBreakpoint {
		t.Fatalf("Reason = %q, want breakpoint", pe.Reason)
	}
}

func TestHandlePausedPlainOtherKeepsReason(t *testing.T) {
	s := NewSession()
	s.handlePaused(map[string]any{"reason": "other", "callFrames": []any{}})
	ev := <-s.Events()
	if pe := ev.Data.(PausedEvent); pe.Reason != PauseReasonOther {
		t.Fatalf("Reason = %q, want other", pe.Reason)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/debug/ -run "TestDiscoverWorkerWS|TestHandlePaused" -v`
Expected: compile error (`DiscoverWorkerWS` undefined); the hitBreakpoints test FAILs once it compiles.

- [ ] **Step 3: Implement discovery**

Create `internal/debug/discover.go`:

```go
package debug

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wrkmn/internal/cfproject"
)

// DefaultFallbackPorts are the inspector ports the fallback scan probes. The
// Vite plugin starts at 9229 and increments per concurrent dev server.
var DefaultFallbackPorts = func() []int {
	ports := make([]int, 0, 21)
	for p := 9229; p <= 9249; p++ {
		ports = append(ports, p)
	}
	return ports
}()

// DiscoverWorkerWS finds the CDP websocket for a named worker.
//
// Primary path: poll debugURL() (populated from the cf dev banner) until
// non-empty, then extract the ws= endpoint from the /__debug page.
// Fallback path: scan fallbackPorts' /json listings for a target whose
// websocket path is exactly "/<workerName>".
//
// TUI-only contract: on timeout the error names both failed paths so the
// status line tells the user exactly what never appeared.
func DiscoverWorkerWS(ctx context.Context, workerName string, debugURL func() string, fallbackPorts []int, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	scanCountdown := 0
	for {
		if u := debugURL(); u != "" {
			ws, err := cfproject.ExtractDebugWS(ctx, u, workerName)
			if err == nil {
				return ws, nil
			}
			lastErr = err
		}
		// Scan the fallback ports every 4th tick (~1s) — cheaper than per-tick.
		if scanCountdown == 0 {
			if ws, ok := scanPorts(fallbackPorts, workerName); ok {
				return ws, nil
			}
			scanCountdown = 4
		}
		scanCountdown--

		select {
		case <-ctx.Done():
			detail := "dev server printed no Debug URL"
			if lastErr != nil {
				detail = lastErr.Error()
			}
			return "", fmt.Errorf("no debug endpoint for %s within %s: %s, and no inspector target matched /%s on the fallback ports", workerName, timeout, detail, workerName)
		case <-ticker.C:
		}
	}
}

// scanPorts probes each port's /json for a target whose websocket path is
// exactly "/<workerName>". Foreign inspectors on the same ports are ignored.
func scanPorts(ports []int, workerName string) (string, bool) {
	for _, port := range ports {
		targets, err := NewInspectorDiscovery(port).Discover()
		if err != nil {
			continue
		}
		for _, t := range targets {
			if strings.HasSuffix(t.WebSocketDebuggerURL, "/"+workerName) {
				return t.WebSocketDebuggerURL, true
			}
		}
	}
	return "", false
}
```

Note for the test's fallback case: `NewInspectorDiscovery(port)` targets `http://localhost:<port>` — the httptest server listens on localhost, so passing its port works directly.

- [ ] **Step 4: hitBreakpoints classification**

In `internal/debug/session.go` `handlePaused`, after `reason := ...`:

```go
	// The cf/Vite inspector reports reason "other" even for breakpoint hits;
	// hitBreakpoints is the authoritative signal (verified 2026-10-19).
	if hits, _ := params["hitBreakpoints"].([]any); len(hits) > 0 {
		reason = PauseReasonBreakpoint
	}
```

- [ ] **Step 5: Rewire enterDebug**

In `internal/ui/table_debug.go`:
- Raise `inspectorWaitTimeout` to `30 * time.Second` (cold Vite starts take several seconds before the banner prints).
- Replace the `w.InspectorPort == 0` gate with nothing (the port concept is gone; keep the `IsCloudflareWorker` and running-process gates).
- Replace the async connect body:

```go
	name := w.Name
	debugURL := func() string { return w.DebugURL }

	return m, func() tea.Msg {
		wsURL, err := debug.DiscoverWorkerWS(context.Background(), name, debugURL, debug.DefaultFallbackPorts, inspectorWaitTimeout)
		if err != nil {
			return debugConnectedMsg{worker: name, err: err}
		}
		target := &debug.InspectorTarget{
			Title:                "Cloudflare Worker",
			WebSocketDebuggerURL: wsURL,
		}
		if err := session.Connect(target); err != nil {
			return debugConnectedMsg{worker: name, err: err}
		}
		return debugConnectedMsg{worker: name, err: nil}
	}
```

(Add `"context"` to the file's imports; drop the now-unused `port` variable.)

- [ ] **Step 6: Run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/debug/ -v && go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/debug/ internal/ui/table_debug.go
git commit -m "debug: attach via cf dev debug endpoint with /json fallback; classify hitBreakpoints"
```

---

### Task 6: Discovery cutover and wrangler excision

**Files:**
- Modify: `internal/worker/discoverer.go` (rewrite), `internal/worker/types.go` (field removals), `internal/ui/remote_logs.go`, `cmd/root.go`
- Delete: `internal/wrangler/` (parser.go, parser_test.go, types.go), `internal/worker/discoverer_test.go` (rewrite in place)
- Modify: `internal/preflight/preflight.go` + `internal/preflight/preflight_test.go` (retire inspector-port checks)

**Interfaces:**
- Consumes: Task 1's `Detect`/`DetectUnmigrated`/`ReadBuildOutput`.
- Produces: `(d *Discoverer) DiscoverWorkers(rootPath string) (workers []*Worker, unmigrated []string, err error)`; `Worker.IsCfProject bool` (and `IsCloudflareWorker()` reading it); Worker struct no longer has `WranglerConfig`, `InspectorPort`, or wrangler-derived account lookups; `internal/wrangler` no longer exists.

- [ ] **Step 1: Write the failing tests**

Rewrite `internal/worker/discoverer_test.go`:

```go
package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverCfProjects(t *testing.T) {
	root := t.TempDir()
	// A cf project with a package.json name.
	writeFile(t, filepath.Join(root, "api", "cloudflare.config.ts"), "export default {}")
	writeFile(t, filepath.Join(root, "api", "package.json"), `{"name":"my-api","scripts":{"dev":"cf dev"}}`)
	// A cf project with no package.json — named by directory.
	writeFile(t, filepath.Join(root, "bare", "cloudflare.config.ts"), "export default {}")
	// An unmigrated wrangler project — reported, not a worker.
	writeFile(t, filepath.Join(root, "legacy", "wrangler.toml"), `name = "legacy"`)
	// A plain directory — ignored entirely.
	writeFile(t, filepath.Join(root, "docs", "readme.md"), "hi")

	workers, unmigrated, err := NewDiscoverer().DiscoverWorkers(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	names := map[string]*Worker{}
	for _, w := range workers {
		names[w.Name] = w
	}
	if len(workers) != 2 {
		t.Fatalf("workers = %d (%v), want 2", len(workers), names)
	}
	if w := names["my-api"]; w == nil || !w.IsCloudflareWorker() || w.Scripts["dev"] != "cf dev" {
		t.Fatalf("my-api = %+v", names["my-api"])
	}
	if w := names["bare"]; w == nil || !w.IsCloudflareWorker() {
		t.Fatalf("bare = %+v", names["bare"])
	}
	if len(unmigrated) != 1 || filepath.Base(unmigrated[0]) != "legacy" {
		t.Fatalf("unmigrated = %v", unmigrated)
	}
}

func TestDiscoverBuildOutputEnrichment(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "api")
	writeFile(t, filepath.Join(dir, "cloudflare.config.ts"), "export default {}")
	writeFile(t, filepath.Join(dir, ".cloudflare", "output", "v0", "config.json"), `{"name":"deployed-api","account_id":"acc1"}`)

	workers, _, err := NewDiscoverer().DiscoverWorkers(root)
	if err != nil || len(workers) != 1 {
		t.Fatalf("discover: %v (%d workers)", err, len(workers))
	}
	if workers[0].Name != "deployed-api" || workers[0].AccountID != "acc1" {
		t.Fatalf("enrichment: %+v", workers[0])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/worker/ -v`
Expected: compile error (signature mismatch / `IsCfProject` undefined).

- [ ] **Step 3: Rewrite the discoverer**

Replace `internal/worker/discoverer.go` wholesale:

```go
package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"wrkmn/internal/cfproject"
)

type Discoverer struct{}

func NewDiscoverer() *Discoverer { return &Discoverer{} }

// DiscoverWorkers scans rootPath's immediate subdirectories. A directory with
// a cloudflare.config.* file is a worker; a directory with only a Wrangler
// config is returned in unmigrated so callers can tell the user to run
// `cf migrate` there.
func (d *Discoverer) DiscoverWorkers(rootPath string) (workers []*Worker, unmigrated []string, err error) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read directory %s: %w", rootPath, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(rootPath, entry.Name())
		if _, ok := cfproject.Detect(dir); ok {
			workers = append(workers, buildWorker(dir))
			continue
		}
		if cfproject.DetectUnmigrated(dir) {
			unmigrated = append(unmigrated, dir)
		}
	}
	return workers, unmigrated, nil
}

// buildWorker assembles a Worker for a detected cf project directory.
// Name precedence: Build Output > package.json name > directory name.
func buildWorker(dir string) *Worker {
	w := &Worker{
		Name:        filepath.Base(dir),
		Path:        dir,
		Status:      StatusStopped,
		IsCfProject: true,
		Scripts:     map[string]string{},
	}

	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg PackageJSON
		if json.Unmarshal(data, &pkg) == nil {
			if pkg.Name != "" {
				w.Name = pkg.Name
			}
			if pkg.Scripts != nil {
				w.Scripts = pkg.Scripts
			}
			w.PackageJSON = &pkg
		}
	}

	if bo, err := cfproject.ReadBuildOutput(dir); err == nil && bo != nil {
		if bo.WorkerName != "" {
			w.Name = bo.WorkerName
		}
		if bo.AccountID != "" {
			w.AccountID = bo.AccountID
		}
	}

	return w
}
```

- [ ] **Step 4: Excise wrangler from the worker model and callers**

- `internal/worker/types.go`: delete `WranglerConfig`, `InspectorPort`, the `wrangler` import, `GetCurrentAccountID` (replace its one caller below), and add:

```go
	// IsCfProject marks a directory with a cloudflare.config.* file.
	IsCfProject bool `json:"is_cf_project,omitempty"`
```

  `IsCloudflareWorker()` becomes `return w.IsCfProject`. `AccountID` stays as a plain field (comment: compose `account_id` > Build Output > `CLOUDFLARE_ACCOUNT_ID` fallback inside the tail client).
- `internal/ui/remote_logs.go`: `r.worker.GetCurrentAccountID()` → `r.worker.AccountID` (the tail client's `ResolveAccountID` already falls back to the environment variable).
- `cmd/root.go`: in the config-worker seeding loop, drop `InspectorPort: proc.InspectorPort,`, add `IsCfProject: isCf,` where `isCf` comes from `cfproject.Detect(proc.WorkingDir)` (false when `WorkingDir` is empty); delete the `worker.EnrichFromWranglerConfig(w)` call and its comment. In the discovery block, adapt to the new return:

```go
		discovered, unmigrated, err := discoverer.DiscoverWorkers(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: discovery failed for %s: %v\n", path, err)
			continue
		}
		for _, dir := range unmigrated {
			fmt.Fprintf(os.Stderr, "Warning: %s is an unmigrated Wrangler project — run 'cf migrate' there to manage it with wrkmn\n", dir)
		}
```

- Delete `internal/worker/discoverer.go`'s old helpers (`EnrichFromWranglerConfig`, `hasWranglerScripts`, `extractPortFromDevScript`, the port regex) — all replaced above.
- `rm -rf internal/wrangler` and fix any straggler import (`grep -rn "internal/wrangler" --include='*.go' .` must come back empty).
- `internal/preflight/preflight.go`: delete the inspector-port occupancy check, the duplicate-port aggregation, and the now-unused `net`/`time` imports if nothing else uses them; delete `TestDuplicateInspectorPortsIsError` and `TestOccupiedInspectorPortIsWarning` from the test file.
- Check `internal/ui/table.go`'s `renderDataRow`: the PORT column reads `w.Port` (still present, now banner-fed) — no change needed; grep for `InspectorPort` across `internal/ ui/ cmd/` and remove any remaining reference (expected: none after the above).

- [ ] **Step 5: Run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./...`
Expected: PASS; `internal/wrangler` gone.

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add -u internal/wrangler internal/worker internal/ui/remote_logs.go internal/ui/table.go cmd/root.go internal/preflight
git add internal/worker/discoverer_test.go
git commit -m "worker: discover cf projects; delete the wrangler TOML layer"
```

---

### Task 7: Pre-flight and validation for the cf world

**Files:**
- Modify: `internal/preflight/preflight.go`, `internal/config/loader.go`
- Test: `internal/preflight/preflight_test.go`, `internal/config/loader_test.go`

**Interfaces:**
- Consumes: `cfproject.DetectUnmigrated`; existing `Issue`/`Severity`.
- Produces: pre-flight errors for `cf` missing from PATH and for unmigrated worker dirs; warning for deprecated `inspector_port:`; the loader no longer infers `type: worker` from `inspector_port`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/preflight/preflight_test.go`:

```go
func TestUnmigratedWorkingDirIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.toml"), []byte(`name = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{BaseDir: dir, Processes: map[string]config.Process{
		"api": {Command: "cf", WorkingDir: dir, Type: config.ProcessTypeWorker},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "cf migrate") {
		t.Fatalf("unmigrated dir not reported:\n%s", issueMessages(issues))
	}
}

func TestDeprecatedInspectorPortWarns(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"api": {Command: "sh", InspectorPort: 9229, Type: config.ProcessTypeWorker},
	}}
	if !findIssue(Check(cfg), "api", SeverityWarning, "inspector_port is ignored") {
		t.Fatal("deprecated inspector_port not warned")
	}
}

func TestCfMissingFromPathIsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // nothing on PATH
	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"api": {Command: "cf", Type: config.ProcessTypeWorker},
	}}
	if !findIssue(Check(cfg), "", SeverityError, "cf CLI not found") {
		t.Fatal("missing cf not reported")
	}
}
```

Append to `internal/config/loader_test.go`:

```go
func TestInspectorPortNoLongerInfersWorkerType(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    inspector_port: 9229
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Processes["api"].Type != ProcessTypeService {
		t.Fatalf("Type = %q, want service (inference removed)", cfg.Processes["api"].Type)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/preflight/ ./internal/config/ -run "TestUnmigrated|TestDeprecatedInspector|TestCfMissing|TestInspectorPortNoLonger" -v`
Expected: FAIL (checks absent; inference still present).

- [ ] **Step 3: Implement**

`internal/preflight/preflight.go` (import `os/exec` already present; add `wrkmn/internal/cfproject`):

Before the per-process loop:

```go
	// Any worker-type process requires the cf CLI.
	hasWorker := false
	for _, proc := range cfg.Processes {
		if proc.Type == config.ProcessTypeWorker {
			hasWorker = true
			break
		}
	}
	if hasWorker {
		if _, err := exec.LookPath("cf"); err != nil {
			issues = append(issues, Issue{SeverityError, "", "cf CLI not found on PATH (install: bun add --global cf)"})
		}
	}
```

Inside the per-process loop:

```go
		if proc.Type == config.ProcessTypeWorker && proc.WorkingDir != "" && cfproject.DetectUnmigrated(proc.WorkingDir) {
			issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("unmigrated Wrangler project (wrangler config present, no cloudflare.config.ts) — run 'cf migrate' in %s", proc.WorkingDir)})
		}
		if proc.InspectorPort > 0 {
			issues = append(issues, Issue{SeverityWarning, name, "inspector_port is ignored since the cf cutover; the debugger discovers its endpoint automatically"})
		}
```

`internal/config/loader.go` `applyDefaults`: the type-default block becomes simply

```go
		// Set default process type
		if proc.Type == "" {
			proc.Type = ProcessTypeService
		}
```

(Check existing loader tests for the old inference — `TestDevModeDefaultsToRemoteBindingsForWorkers` sets `inspector_port` with `type: worker` explicit? If any test relied on inference, give it an explicit `type: worker`.)

- [ ] **Step 4: Run the full suite**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/preflight/ internal/config/
git commit -m "preflight: check cf on PATH and unmigrated projects; deprecate inspector_port"
```

---

### Task 8: Mandatory live smoke (spec gate)

**Files:**
- Create: `test/integration/cfsmoke_test.go` (build tag `cfsmoke`)

**Interfaces:**
- Consumes: everything — this exercises manager launch, banner parsing, `DiscoverWorkerWS`, `Session`, `BreakpointManager.SetBreakpoint(url, line, col, condition)`, `StepController.Resume()`, `Evaluator.EvaluateOnCallFrame(expr, frameID)`.
- Produces: the completion gate. **The cutover is not done until this passes on this machine.**

- [ ] **Step 1: Write the smoke test**

Create `test/integration/cfsmoke_test.go`:

```go
//go:build cfsmoke

// Live smoke for the cf cutover: scaffolds a real `cf init` project, launches
// it through the process manager, attaches the debugger through the same
// discovery path the TUI uses, and drives a breakpoint pause/inspect/resume.
// Requires: cf and bun on PATH, network access for the scaffold's installs.
// Run: go test -tags cfsmoke ./test/integration/ -run TestCfLiveSmoke -v -timeout 300s
package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/debug"
	"wrkmn/internal/process"
	"wrkmn/internal/worker"
)

func TestCfLiveSmoke(t *testing.T) {
	for _, bin := range []string{"cf", "bun"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}

	dir := t.TempDir()
	scaffold := exec.Command("cf", "init", "smoke", "--package-manager", "bun")
	scaffold.Dir = dir
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Fatalf("cf init: %v\n%s", err, out)
	}
	proj := filepath.Join(dir, "smoke")

	w := &worker.Worker{Name: "smoke", Path: proj, Status: worker.StatusStopped, IsCfProject: true}
	mgr := process.NewManager()
	cfg := &config.Process{Command: "cf", Args: []string{"dev"}, WorkingDir: proj, Type: config.ProcessTypeWorker}
	if _, err := mgr.StartProcess(context.Background(), "smoke", cfg, w); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.StopAll()

	// Attach exactly the way enterDebug does.
	wsURL, err := debug.DiscoverWorkerWS(context.Background(), "smoke",
		func() string { return w.DebugURL }, debug.DefaultFallbackPorts, 60*time.Second)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	session := debug.NewSession()
	if err := session.Connect(&debug.InspectorTarget{WebSocketDebuggerURL: wsURL}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Disconnect()

	// Warm the worker so the entrypoint module parses, then set a breakpoint
	// on the line containing "Hello" in the on-disk source (absolute URL,
	// line numbers preserved by the Vite SSR transform — see spec).
	if w.DevServerURL == "" {
		t.Fatal("DevServerURL not populated from banner")
	}
	if _, err := http.Get(w.DevServerURL); err != nil {
		t.Fatalf("warm request: %v", err)
	}
	entry := filepath.Join(proj, "src", "index.ts")
	src, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	line := -1
	for i, l := range strings.Split(string(src), "\n") {
		if strings.Contains(l, "Hello") {
			line = i // CDP lines are 0-indexed
			break
		}
	}
	if line < 0 {
		t.Fatalf("no Hello line in %s", entry)
	}

	bm := debug.NewBreakpointManager(session)
	if _, err := bm.SetBreakpoint(entry, line, 0, ""); err != nil {
		t.Fatalf("set breakpoint: %v", err)
	}

	go http.Get(w.DevServerURL) // the request that should pause

	var pe debug.PausedEvent
	deadline := time.After(30 * time.Second)
wait:
	for {
		select {
		case ev := <-session.Events():
			if ev.Type == debug.EventPaused {
				pe = ev.Data.(debug.PausedEvent)
				break wait
			}
		case <-deadline:
			t.Fatal("never paused on breakpoint")
		}
	}
	if pe.Reason != debug.PauseReasonBreakpoint {
		t.Fatalf("pause reason = %q, want breakpoint", pe.Reason)
	}
	if len(pe.CallFrames) == 0 {
		t.Fatal("paused with no call frames")
	}

	// Evaluate on the paused frame (plain expression — module bindings are
	// SSR-renamed, a documented v1 limitation).
	ev := debug.NewEvaluator(session)
	res, err := ev.EvaluateOnCallFrame("1+41", pe.CallFrames[0].CallFrameID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if fmt.Sprintf("%v", res.Value) != "42" {
		t.Fatalf("eval = %v", res.Value)
	}

	if err := debug.NewStepController(session).Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
}
```

Note to implementer: `CallFrame`'s frame-ID field and `EvaluationResult`'s value field must match `internal/debug/types.go` exactly — check the struct (`CallFrameID`/`ID`, `Value`/`Description`) and adjust the two references; everything else in this test uses verified APIs.

- [ ] **Step 2: Run the gate**

Run: `cd /home/jefflangston/dev/wrkmn && go test -tags cfsmoke ./test/integration/ -run TestCfLiveSmoke -v -timeout 300s`
Expected: PASS (requires cf + bun + network; the test self-skips when missing, but **for the gate it must actually run and pass — a skip does not satisfy the spec**). Also confirm no leaked processes afterward: `pgrep -fl "cf dev" | grep smoke` returns nothing.

- [ ] **Step 3: Manual TUI pass (user-facing check, do not skip reporting it)**

Scaffold a probe project in the scratchpad, run `./wrkmn <dir>`, and verify by hand: `d` starts it, the PORT column fills from the banner, `D` attaches and the debug view opens, a breakpoint set in the source panel pauses a curl, variables render, F5 resumes, `q` quits clean. Record the result in the task report — this is the spec's TUI-only acceptance.

- [ ] **Step 4: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add test/integration/cfsmoke_test.go
git commit -m "test: live cf dev smoke gate (scaffold, attach, breakpoint, resume)"
```

---

### Task 9: Documentation

**Files:**
- Modify: `README.md`, `CLAUDE.md`

- [ ] **Step 1: Update both docs**

Rewrite the affected sections to match shipped behavior (adapt to each file's structure; read both first):

1. **Discovery**: workers are directories with `cloudflare.config.ts` (`.js`/`.mjs`/`.mts` accepted); wrangler.toml projects are reported as unmigrated with a `cf migrate` hint and are not manageable.
2. **Launch**: discovered workers run `cf dev`; compose worker processes get `--mode <m>`/`--local` appended to their configured command (no `--` separator). Remove `inspector_port` from all examples; mark it deprecated/ignored in the config reference.
3. **Dev Modes**: two modes — `local` (`cf dev --local`) and `remote-bindings` (default, plain `cf dev`); full-remote removed (does not exist in cf).
4. **Modes** (new section replacing Environments): `modes:` compose list (default `[development, production]`), `e` selects the mode, empty selection means cf's default (`development`); persisted in `.wrkmn-state.json` as `selected_mode` (legacy `selected_env` migrates automatically).
5. **Debugger**: attaches automatically via the dev server's debug endpoint (banner → `/__debug` → CDP websocket; `/json` port scan fallback) — no configuration; known v1 limitation: module-level bindings are SSR-renamed, so evaluating them by original name in the REPL fails (frame locals work).
6. **Pre-flight**: cf-on-PATH check, unmigrated-project errors, inspector_port deprecation warning; the inspector-port occupancy/duplicate checks are gone.
7. **CLAUDE.md structure tree**: `internal/wrangler/` → `internal/cfproject/` ("cf project detection, banner parsing, debug-ws extraction"); add `internal/debug/discover.go`; update Data Flow steps (banner parsing after launch; debug discovery before CDP connect) and the sample worker-compose (a `cf`-launched worker with `modes:`).
8. Note the smoke gate: `go test -tags cfsmoke ./test/integration/` as the documented way to verify a machine's cf setup end-to-end.

- [ ] **Step 2: Final verification**

Run: `cd /home/jefflangston/dev/wrkmn && gofmt -l internal/ cmd/ test/ | grep -v -f <(git -C . show main:/dev/null 2>/dev/null; true); go vet ./... && go build ./... && go test ./...`
(Practically: `gofmt -l .` must list only the pre-existing dirty files — none that this branch touched; vet/build/tests clean.)

- [ ] **Step 3: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add README.md CLAUDE.md
git commit -m "docs: document the cf CLI cutover (discovery, modes, debugger, pre-flight)"
```

---

## Self-review notes

- **Spec coverage:** Component 1 (`cfproject`) → Task 1. Component 2 (discovery/worker model) → Tasks 3 (fields) + 6 (discovery, excision). Component 3 (launch) → Tasks 2 (mode shrink) + 4 (cf dev, banner). Component 4 (modes) → Task 3. Component 5 (debug attach, hitBreakpoints, SSR-rename limitation) → Tasks 5 + 8. Component 6 (preflight/validation/messaging) → Tasks 6 (retired checks) + 7 (new checks, inference removal). Component 7 (docs) → Task 9. Mandatory live smoke → Task 8 (explicitly: a skip does not satisfy the gate).
- **Type consistency:** `cfproject.Detect/DetectUnmigrated/ReadBuildOutput/ParseBannerLine/ExtractDebugWS` signatures identical in Task 1 (definition) and Tasks 4, 5, 6, 7 (consumers). `buildCfDevArgs(config.DevMode, string)` consistent between Task 4's definition, tests, and both call sites. `DiscoverWorkerWS(ctx, name, debugURL func() string, fallbackPorts []int, timeout)` identical in Task 5 (definition/tests) and Task 8 (smoke). Worker fields (`Modes`, `SelectedMode`, `DevServerURL`, `DebugURL`, `IsCfProject`) introduced in Task 3/6 and consumed by Tasks 4, 5, 8.
- **Green-tree ordering:** Task 2 removes `DevModeRemote` and atomically updates every reference. Task 3 renames fields with all consumers in one commit. Task 4 changes `StartWorker`'s signature together with its two call sites. Task 6 deletes `InspectorPort` only after Task 5 removed its last functional use (the enterDebug gate), and retires the preflight port checks in the same commit as the field.
- **Review Focus:** all five lines carry tests — 1 & 2 in Task 1, 3 & 5 in Task 5, 4 in Task 3.
- **Known debt carried, not expanded:** unsynchronized Worker-field writes (banner parser joins Status under the parked ruling); `EnvSelector` type name retained (labels say mode) to keep Task 3 mechanical.
