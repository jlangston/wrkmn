package debug

import (
	"encoding/json"
	"fmt"
)

// BreakpointManager handles breakpoint operations
type BreakpointManager struct {
	session *Session
}

// NewBreakpointManager creates a new breakpoint manager
func NewBreakpointManager(session *Session) *BreakpointManager {
	return &BreakpointManager{session: session}
}

// SetBreakpoint sets a breakpoint at the specified location
func (m *BreakpointManager) SetBreakpoint(url string, line int, column int, condition string) (*Breakpoint, error) {
	params := map[string]any{
		"lineNumber": line,
		"url":        url,
	}

	if column > 0 {
		params["columnNumber"] = column
	}

	if condition != "" {
		params["condition"] = condition
	}

	result, err := m.session.Call("Debugger.setBreakpointByUrl", params)
	if err != nil {
		return nil, fmt.Errorf("failed to set breakpoint: %w", err)
	}

	var response struct {
		BreakpointID string `json:"breakpointId"`
		Locations    []struct {
			ScriptID   string `json:"scriptId"`
			LineNumber int    `json:"lineNumber"`
			Column     int    `json:"columnNumber"`
		} `json:"locations"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	bp := &Breakpoint{
		ID:         response.BreakpointID,
		ScriptURL:  url,
		LineNumber: line,
		Column:     column,
		Condition:  condition,
		Enabled:    true,
	}

	// If we got a resolved location, use it
	if len(response.Locations) > 0 {
		bp.ScriptID = response.Locations[0].ScriptID
		bp.LineNumber = response.Locations[0].LineNumber
		bp.Column = response.Locations[0].Column
	}

	m.session.state.AddBreakpoint(bp)
	return bp, nil
}

// SetBreakpointOnFunction sets a breakpoint on a function
func (m *BreakpointManager) SetBreakpointOnFunction(objectID string, condition string) (*Breakpoint, error) {
	params := map[string]any{
		"objectId": objectID,
	}

	if condition != "" {
		params["condition"] = condition
	}

	result, err := m.session.Call("Debugger.setBreakpointOnFunctionCall", params)
	if err != nil {
		return nil, fmt.Errorf("failed to set function breakpoint: %w", err)
	}

	var response struct {
		BreakpointID string `json:"breakpointId"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	bp := &Breakpoint{
		ID:        response.BreakpointID,
		Condition: condition,
		Enabled:   true,
	}

	m.session.state.AddBreakpoint(bp)
	return bp, nil
}

// RemoveBreakpoint removes a breakpoint
func (m *BreakpointManager) RemoveBreakpoint(breakpointID string) error {
	params := map[string]any{
		"breakpointId": breakpointID,
	}

	_, err := m.session.Call("Debugger.removeBreakpoint", params)
	if err != nil {
		return fmt.Errorf("failed to remove breakpoint: %w", err)
	}

	m.session.state.RemoveBreakpoint(breakpointID)
	return nil
}

// ToggleBreakpoint toggles a breakpoint's enabled state
func (m *BreakpointManager) ToggleBreakpoint(breakpointID string) (*Breakpoint, error) {
	bp := m.session.state.GetBreakpoint(breakpointID)
	if bp == nil {
		return nil, fmt.Errorf("breakpoint not found: %s", breakpointID)
	}

	if bp.Enabled {
		// Disable by removing and tracking
		if err := m.RemoveBreakpoint(breakpointID); err != nil {
			return nil, err
		}
		bp.Enabled = false
		bp.ID = "" // Clear the CDP ID
		m.session.state.AddBreakpoint(bp)
	} else {
		// Re-enable by re-adding
		newBp, err := m.SetBreakpoint(bp.ScriptURL, bp.LineNumber, bp.Column, bp.Condition)
		if err != nil {
			return nil, err
		}
		bp = newBp
	}

	return bp, nil
}

// GetBreakpoints returns all breakpoints
func (m *BreakpointManager) GetBreakpoints() []*Breakpoint {
	return m.session.state.GetAllBreakpoints()
}

// GetBreakpointsForScript returns breakpoints for a specific script
func (m *BreakpointManager) GetBreakpointsForScript(scriptURL string) []*Breakpoint {
	all := m.session.state.GetAllBreakpoints()
	result := make([]*Breakpoint, 0)

	for _, bp := range all {
		if bp.ScriptURL == scriptURL {
			result = append(result, bp)
		}
	}

	return result
}

// ClearAllBreakpoints removes all breakpoints
func (m *BreakpointManager) ClearAllBreakpoints() error {
	for _, bp := range m.session.state.GetAllBreakpoints() {
		if bp.Enabled && bp.ID != "" {
			if err := m.RemoveBreakpoint(bp.ID); err != nil {
				// Log but continue
			}
		}
	}

	// Clear state
	m.session.state.Breakpoints = make(map[string]*Breakpoint)
	return nil
}

// SetBreakOnExceptions configures exception breakpoints
func (m *BreakpointManager) SetBreakOnExceptions(uncaught, caught bool) error {
	state := "none"
	if uncaught && caught {
		state = "all"
	} else if uncaught {
		state = "uncaught"
	}

	params := map[string]any{
		"state": state,
	}

	_, err := m.session.Call("Debugger.setPauseOnExceptions", params)
	return err
}
