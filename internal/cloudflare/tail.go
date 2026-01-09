package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	cf "github.com/cloudflare/cloudflare-go"
	"github.com/gorilla/websocket"
)

// TailMessage represents a single log message from a Worker tail
type TailMessage struct {
	Timestamp   time.Time       `json:"timestamp"`
	Event       TailEvent       `json:"event"`
	EventType   string          `json:"eventType"` // "fetch", "scheduled", "tail", etc.
	Outcome     string          `json:"outcome"`   // "ok", "exception", "exceededCpu", etc.
	ScriptName  string          `json:"scriptName"`
	Logs        []TailLog       `json:"logs"`
	Exceptions  []TailException `json:"exceptions"`
	Diagnostics TailDiagnostics `json:"diagnostics"`
}

// TailEvent contains details about the event that triggered the Worker
type TailEvent struct {
	Request  *TailRequest  `json:"request,omitempty"`
	Response *TailResponse `json:"response,omitempty"`
}

// TailRequest contains HTTP request information
type TailRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	CF      json.RawMessage   `json:"cf,omitempty"`
}

// TailResponse contains HTTP response information
type TailResponse struct {
	Status int `json:"status"`
}

// TailLog represents a console.log message from the Worker
type TailLog struct {
	Level     string   `json:"level"` // "log", "debug", "info", "warn", "error"
	Message   []string `json:"message"`
	Timestamp time.Time
}

// TailException represents an uncaught exception
type TailException struct {
	Name      string `json:"name"`
	Message   string `json:"message"`
	Timestamp time.Time
}

// TailDiagnostics contains performance diagnostics
type TailDiagnostics struct {
	WallTime  int64 `json:"wallTime"`  // Wall clock time in microseconds
	CPUTime   int64 `json:"cpuTime"`   // CPU time in microseconds
	ScriptTags []string `json:"scriptTags"`
}

// TailFilter specifies which messages to include
type TailFilter struct {
	Status   []string // "ok", "error", "canceled"
	Method   []string // HTTP methods to include
	Search   string   // Text search filter
	IPFilter []string // IP addresses to include
	Sampling float64  // Sampling rate (0.0-1.0)
}

// TailSession manages a WebSocket connection to stream Worker logs
type TailSession struct {
	client      *Client
	workerName  string
	accountID   string
	environment string
	filter      TailFilter

	conn      *websocket.Conn
	messages  chan TailMessage
	errors    chan error
	done      chan struct{}
	closeOnce sync.Once

	tailID string
	url    string

	mu        sync.RWMutex
	connected bool
}

// NewTailSession creates a new tail session for a Worker
func NewTailSession(client *Client, accountID, workerName, environment string) *TailSession {
	return &TailSession{
		client:      client,
		workerName:  workerName,
		accountID:   accountID,
		environment: environment,
		messages:    make(chan TailMessage, 100),
		errors:      make(chan error, 10),
		done:        make(chan struct{}),
	}
}

// SetFilter sets the filter for tail messages
func (t *TailSession) SetFilter(filter TailFilter) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.filter = filter
}

// Start begins streaming logs from the Worker
func (t *TailSession) Start(ctx context.Context) error {
	resolvedAccountID, err := ResolveAccountID(t.accountID)
	if err != nil {
		return err
	}

	// Create a tail using the Cloudflare API
	rc := cf.AccountIdentifier(resolvedAccountID)

	scriptName := t.workerName
	if t.environment != "" && t.environment != "production" {
		// For environments, the script name is typically "scriptName" or might need env suffix
		// The actual behavior depends on the wrangler setup
		scriptName = t.workerName
	}

	// Create the tail session
	tail, err := t.client.api.StartWorkersTail(ctx, rc, scriptName)
	if err != nil {
		return fmt.Errorf("starting worker tail: %w", err)
	}

	t.tailID = tail.ID
	t.url = tail.URL

	// Connect via WebSocket
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, t.url, nil)
	if err != nil {
		return fmt.Errorf("connecting to tail WebSocket: %w", err)
	}

	t.conn = conn
	t.mu.Lock()
	t.connected = true
	t.mu.Unlock()

	// Start reading messages
	go t.readLoop()

	return nil
}

// readLoop continuously reads messages from the WebSocket
func (t *TailSession) readLoop() {
	defer func() {
		t.mu.Lock()
		t.connected = false
		t.mu.Unlock()
		close(t.messages)
	}()

	for {
		select {
		case <-t.done:
			return
		default:
		}

		_, data, err := t.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			select {
			case t.errors <- fmt.Errorf("reading websocket: %w", err):
			default:
			}
			return
		}

		// Parse the message
		var msg TailMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			// Try parsing as an array of messages (sometimes batched)
			var msgs []TailMessage
			if err := json.Unmarshal(data, &msgs); err != nil {
				select {
				case t.errors <- fmt.Errorf("parsing tail message: %w", err):
				default:
				}
				continue
			}
			for _, m := range msgs {
				if t.matchesFilter(m) {
					select {
					case t.messages <- m:
					case <-t.done:
						return
					}
				}
			}
			continue
		}

		if t.matchesFilter(msg) {
			select {
			case t.messages <- msg:
			case <-t.done:
				return
			}
		}
	}
}

// matchesFilter checks if a message passes the current filter
func (t *TailSession) matchesFilter(msg TailMessage) bool {
	t.mu.RLock()
	filter := t.filter
	t.mu.RUnlock()

	// Filter by outcome/status
	if len(filter.Status) > 0 {
		matched := false
		for _, status := range filter.Status {
			if status == "ok" && msg.Outcome == "ok" {
				matched = true
				break
			}
			if status == "error" && (msg.Outcome == "exception" || msg.Outcome == "exceededCpu" || msg.Outcome == "exceededMemory") {
				matched = true
				break
			}
			if status == msg.Outcome {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by HTTP method
	if len(filter.Method) > 0 && msg.Event.Request != nil {
		matched := false
		for _, method := range filter.Method {
			if msg.Event.Request.Method == method {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Text search filter - check URL, logs, and exceptions
	if filter.Search != "" {
		matched := false

		// Check URL
		if msg.Event.Request != nil {
			if containsIgnoreCase(msg.Event.Request.URL, filter.Search) {
				matched = true
			}
		}

		// Check logs
		if !matched {
			for _, log := range msg.Logs {
				for _, m := range log.Message {
					if containsIgnoreCase(m, filter.Search) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}

		// Check exceptions
		if !matched {
			for _, exc := range msg.Exceptions {
				if containsIgnoreCase(exc.Message, filter.Search) || containsIgnoreCase(exc.Name, filter.Search) {
					matched = true
					break
				}
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// Messages returns the channel for receiving tail messages
func (t *TailSession) Messages() <-chan TailMessage {
	return t.messages
}

// Errors returns the channel for receiving errors
func (t *TailSession) Errors() <-chan error {
	return t.errors
}

// IsConnected returns true if the session is currently connected
func (t *TailSession) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connected
}

// Stop closes the tail session
func (t *TailSession) Stop(ctx context.Context) error {
	t.closeOnce.Do(func() {
		close(t.done)

		if t.conn != nil {
			t.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			t.conn.Close()
		}

		// Delete the tail from Cloudflare
		if t.tailID != "" && t.client != nil {
			resolvedAccountID, _ := ResolveAccountID(t.accountID)
			if resolvedAccountID != "" {
				rc := cf.AccountIdentifier(resolvedAccountID)
				_ = t.client.api.DeleteWorkersTail(ctx, rc, t.workerName, t.tailID)
			}
		}
	})

	return nil
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive contains
	sl := len(s)
	subl := len(substr)
	if subl > sl {
		return false
	}

	for i := 0; i <= sl-subl; i++ {
		match := true
		for j := 0; j < subl; j++ {
			sc := s[i+j]
			tc := substr[j]
			// Convert to lowercase
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// FormatTailMessage formats a tail message for display
func FormatTailMessage(msg TailMessage) string {
	ts := msg.Timestamp.Format("15:04:05")
	outcome := msg.Outcome
	if outcome == "ok" {
		outcome = "OK"
	} else {
		outcome = "ERROR"
	}

	var method, url string
	if msg.Event.Request != nil {
		method = msg.Event.Request.Method
		url = msg.Event.Request.URL
	}

	cpuTime := ""
	if msg.Diagnostics.CPUTime > 0 {
		cpuTime = fmt.Sprintf("(%dms CPU)", msg.Diagnostics.CPUTime/1000)
	}

	return fmt.Sprintf("%s [%s] %s %s %s", ts, outcome, method, url, cpuTime)
}
