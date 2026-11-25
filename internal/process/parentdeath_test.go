package process

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"wrkmn/internal/config"
)

// TestHelperParentProcess is not a real test: it is re-executed as a
// subprocess by TestChildDiesWithParent. It starts a managed child the way
// wrkmn does, prints the child's PID, and then hangs until it is killed.
func TestHelperParentProcess(t *testing.T) {
	if os.Getenv("WRKMN_PARENT_DEATH_HELPER") != "1" {
		t.Skip("helper process, only runs re-exec'd")
	}
	m := NewManager()
	cfg := &config.Process{Command: "sleep", Args: []string{"60"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "orphan-candidate", cfg, nil)
	if err != nil {
		fmt.Printf("HELPER_ERR %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("CHILD_PID %d\n", p.Pid())
	// Hang until the parent test SIGKILLs us — the point is that cleanup
	// code never runs, and the kernel's Pdeathsig still reaps the child.
	select {}
}

// TestChildDiesWithParent proves the Pdeathsig contract: when the wrkmn
// process dies WITHOUT any cleanup (SIGKILL — no StopAll, no deferred
// teardown), its managed children are SIGTERMed by the kernel. This is what
// guarantees side-effect-bearing rows (e.g. a public tailscale funnel) cannot
// outlive the TUI.
func TestChildDiesWithParent(t *testing.T) {
	helper := exec.Command(os.Args[0], "-test.run", "^TestHelperParentProcess$", "-test.v")
	helper.Env = append(os.Environ(), "WRKMN_PARENT_DEATH_HELPER=1")
	stdout, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = helper.Process.Kill()
		_, _ = helper.Process.Wait()
	}()

	// Read the child PID the helper prints.
	childPID := 0
	scanner := bufio.NewScanner(stdout)
	deadline := time.After(15 * time.Second)
	pidCh := make(chan int, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if rest, ok := strings.CutPrefix(line, "CHILD_PID "); ok {
				if pid, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
					pidCh <- pid
					return
				}
			}
			if strings.HasPrefix(line, "HELPER_ERR") {
				pidCh <- 0
				return
			}
		}
		pidCh <- 0
	}()
	select {
	case childPID = <-pidCh:
	case <-deadline:
		t.Fatal("helper never reported a child PID")
	}
	if childPID <= 0 {
		t.Fatal("helper failed to start its child")
	}

	// Sanity: the child is alive while the helper lives.
	if err := syscall.Kill(childPID, 0); err != nil {
		t.Fatalf("child %d not alive before parent death: %v", childPID, err)
	}

	// Kill the helper the ungraceful way — no cleanup code runs.
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	_, _ = helper.Process.Wait()

	// The kernel should deliver SIGTERM (Pdeathsig) to the child; sleep dies
	// immediately on SIGTERM.
	waitUntil := time.Now().Add(5 * time.Second)
	for time.Now().Before(waitUntil) {
		if err := syscall.Kill(childPID, 0); err == syscall.ESRCH {
			return // child gone — contract holds
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Best-effort cleanup before failing so we don't leak the sleep.
	_ = syscall.Kill(childPID, syscall.SIGKILL)
	t.Fatalf("child %d survived its parent's SIGKILL — Pdeathsig not effective", childPID)
}
