package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// CDPMessage represents a Chrome DevTools Protocol message
type CDPMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params map[string]any  `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *CDPError       `json:"error,omitempty"`
}

// CDPError represents a CDP error
type CDPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *CDPError) Error() string {
	return fmt.Sprintf("CDP error %d: %s", e.Code, e.Message)
}

// Session represents a debug session with a V8 inspector
type Session struct {
	conn      *websocket.Conn
	state     *SessionState
	msgID     int64
	pending   map[int64]chan *CDPMessage
	pendingMu sync.Mutex
	events    chan DebugEvent
	ctx       context.Context
	cancel    context.CancelFunc
	target    *InspectorTarget
}

// NewSession creates a new debug session
func NewSession() *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		state:   NewSessionState(),
		pending: make(map[int64]chan *CDPMessage),
		events:  make(chan DebugEvent, 100),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Connect establishes a WebSocket connection to the inspector
func (s *Session) Connect(target *InspectorTarget) error {
	s.target = target
	s.state.SetState(StateConnecting)

	dialer := websocket.Dialer{}

	// Origin is required by miniflare's inspector proxy (the server behind
	// cf dev's ws://localhost:9229/<worker> endpoint). Its upgrade validation
	// (InspectorProxyController#validateDevToolsWebSocketUpgradeRequest in
	// miniflare's inspector-proxy-controller.ts) demands an allowed Origin
	// header whenever the handshake carries a User-Agent, and on failure it
	// completes the 101 upgrade and then closes with a bare close frame —
	// which gorilla surfaces as "close 1005 (no status)" within milliseconds.
	// gorilla writes its handshake via net/http's Request.Write, which always
	// injects "User-Agent: Go-http-client/1.1" when none is set, so without
	// an Origin every connect is rejected ("Expected `Origin` header").
	// "http://localhost" is the exact value miniflare itself substitutes for
	// UA-less CDP clients (e.g. Node's `ws`) and its hostname is permanently
	// in the proxy's ALLOWED_ORIGIN_HOSTNAMES list. Verified byte-for-byte
	// against @cloudflare/vite-plugin 2.0.0-beta.v.4 / miniflare
	// 5.20261019.2-beta on 2026-10-24: identical handshakes with and without
	// this header fail and succeed respectively.
	header := http.Header{"Origin": {"http://localhost"}}
	conn, _, err := dialer.DialContext(s.ctx, target.WebSocketDebuggerURL, header)
	if err != nil {
		s.state.SetState(StateDisconnected)
		return fmt.Errorf("failed to connect to inspector: %w", err)
	}

	s.conn = conn
	s.state.SetState(StateConnected)

	// Start message receiver
	go s.receiveMessages()

	// Enable required domains
	if err := s.enableDomains(); err != nil {
		s.Disconnect()
		return fmt.Errorf("failed to enable domains: %w", err)
	}

	s.state.SetState(StateRunning)
	return nil
}

// Disconnect closes the session
func (s *Session) Disconnect() {
	s.cancel()
	if s.conn != nil {
		s.conn.Close()
	}

	// Clean up pending response channels to unblock any waiting callers
	s.pendingMu.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.pendingMu.Unlock()

	s.state.SetState(StateDisconnected)
}

// State returns the current session state
func (s *Session) State() *SessionState {
	return s.state
}

// Events returns the event channel
func (s *Session) Events() <-chan DebugEvent {
	return s.events
}

// Done returns a channel that is closed when the session is disconnected.
// Consumers reading from Events() should also select on Done() so they unblock
// when the session shuts down instead of leaking a goroutine.
func (s *Session) Done() <-chan struct{} {
	return s.ctx.Done()
}

// enableDomains enables required CDP domains
func (s *Session) enableDomains() error {
	// Enable Debugger domain
	if _, err := s.Call("Debugger.enable", nil); err != nil {
		return err
	}

	// Enable Runtime domain
	if _, err := s.Call("Runtime.enable", nil); err != nil {
		return err
	}

	// Enable Console domain (for console.log)
	if _, err := s.Call("Runtime.runIfWaitingForDebugger", nil); err != nil {
		// Ignore error - this is optional
	}

	return nil
}

// Call sends a CDP command and waits for the response
func (s *Session) Call(method string, params map[string]any) (json.RawMessage, error) {
	id := atomic.AddInt64(&s.msgID, 1)

	msg := CDPMessage{
		ID:     id,
		Method: method,
		Params: params,
	}

	// Create response channel
	respChan := make(chan *CDPMessage, 1)
	s.pendingMu.Lock()
	s.pending[id] = respChan
	s.pendingMu.Unlock()

	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
	}()

	// Send message
	if err := s.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for response
	select {
	case resp, ok := <-respChan:
		if !ok {
			// Channel was closed (session disconnected)
			return nil, fmt.Errorf("session disconnected")
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// receiveMessages handles incoming WebSocket messages
func (s *Session) receiveMessages() {
	const readTimeout = 5 * time.Second

	for {
		// Check context before reading
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Set read deadline to allow periodic context checks
		s.conn.SetReadDeadline(time.Now().Add(readTimeout))

		var msg CDPMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			// Check if context was cancelled
			select {
			case <-s.ctx.Done():
				return
			default:
			}

			// Timeout is expected when no messages have arrived recently;
			// go back around and check ctx/deadline again.
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}

			// Any other read error - including a normal/going-away close and
			// an abnormal closure (e.g. the peer tearing down the TCP
			// connection without a close frame) - means this connection is
			// permanently broken. gorilla panics ("repeated read on failed
			// websocket connection") if we call ReadJSON again, so treat the
			// session as dead: log once, tear it down exactly once, and stop
			// reading. Disconnect() cancels ctx, which is how Done() (and in
			// turn the UI's listenDebugEvents) learns the session is gone.
			log.Printf("debug session read error: %v; closing session", err)
			s.Disconnect()
			return
		}

		// Handle response to pending call
		if msg.ID > 0 {
			s.pendingMu.Lock()
			if ch, ok := s.pending[msg.ID]; ok {
				select {
				case ch <- &msg:
				default:
					// Channel already has a message or was closed
				}
			}
			s.pendingMu.Unlock()
			continue
		}

		// Handle event
		s.handleEvent(msg.Method, msg.Params)
	}
}

// handleEvent processes CDP events
func (s *Session) handleEvent(method string, params map[string]any) {
	switch method {
	case "Debugger.paused":
		s.handlePaused(params)

	case "Debugger.resumed":
		s.state.ClearCallStack()
		s.state.SetState(StateRunning)
		s.sendEvent(EventResumed, nil)

	case "Debugger.scriptParsed":
		s.handleScriptParsed(params)

	case "Runtime.consoleAPICalled":
		s.handleConsole(params)

	case "Runtime.exceptionThrown":
		s.handleException(params)
	}
}

// handlePaused processes the Debugger.paused event
func (s *Session) handlePaused(params map[string]any) {
	reason := PauseReason(getString(params, "reason"))

	// The cf/Vite inspector reports reason "other" even for breakpoint hits;
	// hitBreakpoints is the authoritative signal (verified 2026-10-19).
	if hits, _ := params["hitBreakpoints"].([]any); len(hits) > 0 {
		reason = PauseReasonBreakpoint
	}

	// Parse call frames
	rawFrames, _ := params["callFrames"].([]any)
	frames := make([]CallFrame, 0, len(rawFrames))

	for _, rf := range rawFrames {
		if fm, ok := rf.(map[string]any); ok {
			frame := parseCallFrame(fm)
			frames = append(frames, frame)
		}
	}

	s.state.SetCallStack(frames, reason)
	s.state.SetState(StatePaused)

	s.sendEvent(EventPaused, PausedEvent{
		Reason:     reason,
		CallFrames: frames,
		Data:       params,
	})
}

// handleScriptParsed processes the Debugger.scriptParsed event
func (s *Session) handleScriptParsed(params map[string]any) {
	script := &Script{
		ID:  getString(params, "scriptId"),
		URL: getString(params, "url"),
	}

	s.state.AddScript(script)
	s.sendEvent(EventScriptParsed, script)
}

// handleConsole processes console API calls
func (s *Session) handleConsole(params map[string]any) {
	s.sendEvent(EventConsole, params)
}

// handleException processes exception events
func (s *Session) handleException(params map[string]any) {
	s.sendEvent(EventException, params)
}

// sendEvent sends an event to the events channel
func (s *Session) sendEvent(eventType string, data any) {
	select {
	case s.events <- DebugEvent{Type: eventType, Data: data}:
	default:
		// Channel full, log warning about dropped event
		log.Printf("warning: debug event channel full, dropping event: %s", eventType)
	}
}

// Helper functions

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func parseCallFrame(m map[string]any) CallFrame {
	loc := CallFrame{
		ID:           getString(m, "callFrameId"),
		FunctionName: getString(m, "functionName"),
	}

	if location, ok := m["location"].(map[string]any); ok {
		loc.Location = Location{
			ScriptID:   getString(location, "scriptId"),
			LineNumber: getInt(location, "lineNumber"),
			Column:     getInt(location, "columnNumber"),
		}
	}

	loc.URL = getString(m, "url")

	// Parse scope chain
	if scopeChain, ok := m["scopeChain"].([]any); ok {
		for _, sc := range scopeChain {
			if sm, ok := sc.(map[string]any); ok {
				scope := Scope{
					Type: getString(sm, "type"),
					Name: getString(sm, "name"),
				}
				if obj, ok := sm["object"].(map[string]any); ok {
					scope.Object = parseRemoteObject(obj)
				}
				loc.ScopeChain = append(loc.ScopeChain, scope)
			}
		}
	}

	// Parse this
	if thisObj, ok := m["this"].(map[string]any); ok {
		loc.This = parseRemoteObject(thisObj)
	}

	return loc
}

func parseRemoteObject(m map[string]any) RemoteObject {
	return RemoteObject{
		Type:        getString(m, "type"),
		Subtype:     getString(m, "subtype"),
		ClassName:   getString(m, "className"),
		Value:       m["value"],
		Description: getString(m, "description"),
		ObjectID:    getString(m, "objectId"),
	}
}
