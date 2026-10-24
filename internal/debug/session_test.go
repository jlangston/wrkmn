package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestSession_NewSession(t *testing.T) {
	s := NewSession()
	assert.NotNil(t, s)
	assert.NotNil(t, s.state)
	assert.NotNil(t, s.pending)
	assert.NotNil(t, s.events)
	assert.NotNil(t, s.ctx)
	assert.NotNil(t, s.cancel)
}

func TestSession_Disconnect_CleansPendingChannels(t *testing.T) {
	s := NewSession()

	// Simulate pending channels
	ch1 := make(chan *CDPMessage, 1)
	ch2 := make(chan *CDPMessage, 1)
	ch3 := make(chan *CDPMessage, 1)

	s.pendingMu.Lock()
	s.pending[1] = ch1
	s.pending[2] = ch2
	s.pending[3] = ch3
	s.pendingMu.Unlock()

	// Disconnect should close all pending channels
	s.Disconnect()

	// Verify channels are closed (reads return immediately with zero value)
	select {
	case _, ok := <-ch1:
		assert.False(t, ok, "channel 1 should be closed")
	default:
		t.Error("channel 1 should be readable (and closed)")
	}

	select {
	case _, ok := <-ch2:
		assert.False(t, ok, "channel 2 should be closed")
	default:
		t.Error("channel 2 should be readable (and closed)")
	}

	select {
	case _, ok := <-ch3:
		assert.False(t, ok, "channel 3 should be closed")
	default:
		t.Error("channel 3 should be readable (and closed)")
	}

	// Pending map should be empty
	s.pendingMu.Lock()
	assert.Empty(t, s.pending)
	s.pendingMu.Unlock()
}

func TestSession_Disconnect_MultipleCalls(t *testing.T) {
	s := NewSession()

	// Multiple disconnect calls should not panic
	assert.NotPanics(t, func() {
		s.Disconnect()
		s.Disconnect()
		s.Disconnect()
	})
}

func TestSession_PendingMap_ThreadSafe(t *testing.T) {
	s := NewSession()

	var wg sync.WaitGroup
	numGoroutines := 50
	opsPerGoroutine := 100

	// Concurrent access to pending map
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				msgID := int64(id*1000 + j)

				// Add pending
				s.pendingMu.Lock()
				s.pending[msgID] = make(chan *CDPMessage, 1)
				s.pendingMu.Unlock()

				// Check pending
				s.pendingMu.Lock()
				_, exists := s.pending[msgID]
				s.pendingMu.Unlock()
				_ = exists

				// Remove pending
				s.pendingMu.Lock()
				delete(s.pending, msgID)
				s.pendingMu.Unlock()
			}
		}(i)
	}

	// Should not race or panic
	assert.NotPanics(t, func() {
		wg.Wait()
	})
}

func TestSession_Events_NonBlocking(t *testing.T) {
	s := NewSession()

	// Fill the event buffer (100 events)
	for i := 0; i < 100; i++ {
		s.sendEvent("test", i)
	}

	// Additional sends should not block (they get dropped with warning)
	done := make(chan bool)
	go func() {
		s.sendEvent("overflow", nil)
		done <- true
	}()

	select {
	case <-done:
		// Good, didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("sendEvent blocked when channel full")
	}
}

func TestSession_State(t *testing.T) {
	s := NewSession()

	state := s.State()
	assert.NotNil(t, state)
	assert.Equal(t, StateDisconnected, state.GetState())
}

func TestSession_EventsChannel(t *testing.T) {
	s := NewSession()

	events := s.Events()
	assert.NotNil(t, events)

	// Send an event
	s.sendEvent(EventPaused, nil)

	// Should receive it
	select {
	case event := <-events:
		assert.Equal(t, EventPaused, event.Type)
	case <-time.After(100 * time.Millisecond):
		t.Error("event not received")
	}
}

func TestSessionState_ThreadSafe(t *testing.T) {
	state := NewSessionState()

	var wg sync.WaitGroup

	// Concurrent state changes
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state.SetState(StateRunning)
				state.GetState()
				state.SetState(StatePaused)
				state.GetState()
			}
		}()
	}

	// Concurrent script access
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state.AddScript(&Script{ID: "script-" + string(rune(id))})
				state.GetAllScripts()
			}
		}(i)
	}

	// Should not race or panic
	assert.NotPanics(t, func() {
		wg.Wait()
	})
}

func TestSessionState_GetAllScripts_ReturnsCopy(t *testing.T) {
	state := NewSessionState()

	// Add some scripts
	state.AddScript(&Script{ID: "1", URL: "a.js"})
	state.AddScript(&Script{ID: "2", URL: "b.js"})

	// Get all scripts
	scripts := state.GetAllScripts()
	assert.Len(t, scripts, 2)

	// Modifying the returned slice shouldn't affect the state
	scripts[0] = nil
	scripts = scripts[:1]

	// Get again should still have 2 scripts
	scripts2 := state.GetAllScripts()
	assert.Len(t, scripts2, 2)
}

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

// TestSession_AbnormalCloseTerminatesSession reproduces the production defect
// found by the live CDP smoke test (see .superpowers/sdd/2026-10-19-cf-cutover/
// task-8-report.md): a peer that tears the WebSocket connection down
// abnormally (no close frame, e.g. CDP close code 1005 "no status" as seen
// against the real `cf dev` inspector) must cause receiveMessages to treat
// the connection as dead -- not retry ReadJSON forever, which trips
// gorilla's "repeated read on failed websocket connection" panic guard and
// also leaves Connect()'s enableDomains() call parked on <-respChan forever
// since nothing ever cancels the session context.
//
// This test accepts a WebSocket upgrade then immediately closes the
// underlying TCP connection without sending a close frame (gorilla's
// (*Conn).Close() on the server side is documented to close "without
// sending or waiting for a close message"). It asserts, within a bounded
// wait, that the session terminates cleanly: no panic, Done() closes, and
// Connect() returns instead of hanging, leaving the session disconnected.
func TestSession_AbnormalCloseTerminatesSession(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Abruptly tear down the TCP connection without a close frame --
		// mirrors the real cf dev inspector's instant abnormal close (1005).
		conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	s := NewSession()
	target := &InspectorTarget{WebSocketDebuggerURL: wsURL}

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- s.Connect(target)
	}()

	select {
	case <-s.Done():
		// Session torn down, as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("session did not terminate within 5s of an abnormal close")
	}

	// Connect() may be blocked inside enableDomains() waiting on a CDP
	// response that will never arrive; it must unblock via ctx cancellation
	// instead of hanging forever.
	select {
	case <-connectDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Connect() did not return within 5s after session termination")
	}

	if got := s.State().GetState(); got != StateDisconnected {
		t.Fatalf("state = %v, want %v", got, StateDisconnected)
	}
}
