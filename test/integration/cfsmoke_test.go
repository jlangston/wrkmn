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
	res, err := ev.EvaluateOnCallFrame("1+41", pe.CallFrames[0].ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if fmt.Sprintf("%v", res.Result.Value) != "42" {
		t.Fatalf("eval = %v", res.Result.Value)
	}

	if err := debug.NewStepController(session).Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
}
