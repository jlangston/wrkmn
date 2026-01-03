package debug

import (
	"sync"
)

// DebugState represents the current debugging state
type DebugState int

const (
	StateDisconnected DebugState = iota
	StateConnecting
	StateConnected
	StateRunning
	StatePaused
)

func (s DebugState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}

// Breakpoint represents a breakpoint in the source code
type Breakpoint struct {
	ID         string // CDP breakpoint ID
	ScriptID   string // CDP script ID
	ScriptURL  string // Script URL (file path)
	LineNumber int    // 0-indexed line number
	Column     int    // 0-indexed column (optional)
	Condition  string // Conditional expression (optional)
	Enabled    bool
	HitCount   int
}

// Location represents a position in source code
type Location struct {
	ScriptID   string
	LineNumber int
	Column     int
}

// CallFrame represents a single frame in the call stack
type CallFrame struct {
	ID           string     // CDP call frame ID
	FunctionName string
	Location     Location
	URL          string
	ScopeChain   []Scope
	This         RemoteObject
}

// Scope represents a scope in the call frame
type Scope struct {
	Type   string       // global, local, closure, etc.
	Name   string       // Optional scope name
	Object RemoteObject // The object representing the scope
}

// RemoteObject represents a JavaScript object in the debuggee
type RemoteObject struct {
	Type        string // object, function, undefined, string, number, boolean, symbol, bigint
	Subtype     string // array, null, node, regexp, date, map, set, weakmap, weakset, iterator, generator, error, proxy, promise, typedarray, arraybuffer, dataview, webassemblymemory, wasmvalue
	ClassName   string // Object class name
	Value       any    // The value (for primitives)
	Description string // String representation
	ObjectID    string // CDP object ID (for objects)
	Preview     *ObjectPreview
}

// ObjectPreview represents a preview of an object's properties
type ObjectPreview struct {
	Type        string
	Subtype     string
	Description string
	Overflow    bool
	Properties  []PropertyPreview
}

// PropertyPreview represents a preview of an object property
type PropertyPreview struct {
	Name  string
	Type  string
	Value string
}

// Script represents a loaded JavaScript script
type Script struct {
	ID     string
	URL    string
	Source string // Source code (fetched on demand)
	Lines  int
	Hash   string
}

// ExceptionDetails represents exception information
type ExceptionDetails struct {
	ExceptionID  int
	Text         string
	LineNumber   int
	ColumnNumber int
	ScriptID     string
	URL          string
	Exception    *RemoteObject
}

// EvaluationResult represents the result of evaluating an expression
type EvaluationResult struct {
	Result           RemoteObject
	ExceptionDetails *ExceptionDetails
}

// PauseReason indicates why execution was paused
type PauseReason string

const (
	PauseReasonBreakpoint  PauseReason = "breakpoint"
	PauseReasonStep        PauseReason = "step"
	PauseReasonException   PauseReason = "exception"
	PauseReasonDebugger    PauseReason = "debuggerStatement"
	PauseReasonOther       PauseReason = "other"
)

// PausedEvent represents a paused event from CDP
type PausedEvent struct {
	Reason     PauseReason
	CallFrames []CallFrame
	Data       map[string]any // Additional data (breakpoint ID, exception, etc.)
}

// SessionState holds the current state of a debug session
type SessionState struct {
	State       DebugState
	Breakpoints map[string]*Breakpoint // ID -> Breakpoint
	Scripts     map[string]*Script     // ID -> Script
	CallStack   []CallFrame
	PauseReason PauseReason
	mu          sync.RWMutex
}

// NewSessionState creates a new session state
func NewSessionState() *SessionState {
	return &SessionState{
		State:       StateDisconnected,
		Breakpoints: make(map[string]*Breakpoint),
		Scripts:     make(map[string]*Script),
	}
}

// SetState updates the debug state
func (s *SessionState) SetState(state DebugState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// GetState returns the current debug state
func (s *SessionState) GetState() DebugState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// AddBreakpoint adds a breakpoint
func (s *SessionState) AddBreakpoint(bp *Breakpoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Breakpoints[bp.ID] = bp
}

// RemoveBreakpoint removes a breakpoint
func (s *SessionState) RemoveBreakpoint(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Breakpoints, id)
}

// GetBreakpoint returns a breakpoint by ID
func (s *SessionState) GetBreakpoint(id string) *Breakpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Breakpoints[id]
}

// GetAllBreakpoints returns all breakpoints
func (s *SessionState) GetAllBreakpoints() []*Breakpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bps := make([]*Breakpoint, 0, len(s.Breakpoints))
	for _, bp := range s.Breakpoints {
		bps = append(bps, bp)
	}
	return bps
}

// AddScript adds a script
func (s *SessionState) AddScript(script *Script) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Scripts[script.ID] = script
}

// GetScript returns a script by ID
func (s *SessionState) GetScript(id string) *Script {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Scripts[id]
}

// GetScriptByURL returns a script by URL
func (s *SessionState) GetScriptByURL(url string) *Script {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, script := range s.Scripts {
		if script.URL == url {
			return script
		}
	}
	return nil
}

// SetCallStack updates the call stack
func (s *SessionState) SetCallStack(frames []CallFrame, reason PauseReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CallStack = frames
	s.PauseReason = reason
}

// GetCallStack returns the current call stack
func (s *SessionState) GetCallStack() []CallFrame {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CallStack
}

// ClearCallStack clears the call stack (when resuming)
func (s *SessionState) ClearCallStack() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CallStack = nil
	s.PauseReason = ""
}

// DebugEvent represents an event from the debug session
type DebugEvent struct {
	Type string
	Data any
}

// Event types
const (
	EventPaused       = "paused"
	EventResumed      = "resumed"
	EventScriptParsed = "scriptParsed"
	EventConsole      = "console"
	EventException    = "exception"
)

// ResumedEvent represents a resumed event (execution continued)
type ResumedEvent struct{}

// ScriptParsedEvent represents a script parsed event from CDP
type ScriptParsedEvent struct {
	ScriptID string
	URL      string
	StartLine int
	EndLine   int
	Hash     string
}
