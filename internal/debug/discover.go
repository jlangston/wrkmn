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
		// Scan the fallback ports immediately on the first iteration, then every
		// 4th tick thereafter (~1s) — cheaper than per-tick.
		if scanCountdown == 0 {
			if ws, ok := scanPorts(ctx, fallbackPorts, workerName); ok {
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

// scanPortsProbeTimeout bounds a single port's /json probe. Localhost answers
// in milliseconds when an inspector is actually listening, so this only
// costs real time against a stuck/unresponsive port — and even then it's
// capped well under DiscoverWorkerWS's overall timeout instead of riding the
// client's own (much longer) timeout.
const scanPortsProbeTimeout = 500 * time.Millisecond

// scanPorts probes each port's /json for a target whose websocket path is
// exactly "/<workerName>". Foreign inspectors on the same ports are ignored.
// Each probe runs under its own short-lived child of ctx so one stuck port
// can't stall the scan past ctx's deadline; ctx is also checked between
// ports so a cancelled/expired parent stops the scan immediately instead of
// working through the rest of the port list.
func scanPorts(ctx context.Context, ports []int, workerName string) (string, bool) {
	for _, port := range ports {
		if ctx.Err() != nil {
			return "", false
		}
		probeCtx, cancel := context.WithTimeout(ctx, scanPortsProbeTimeout)
		targets, err := NewInspectorDiscovery(port).DiscoverContext(probeCtx)
		cancel()
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
