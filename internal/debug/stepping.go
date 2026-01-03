package debug

import (
	"fmt"
)

// StepController handles stepping operations
type StepController struct {
	session *Session
}

// NewStepController creates a new step controller
func NewStepController(session *Session) *StepController {
	return &StepController{session: session}
}

// Resume continues execution (F5)
func (s *StepController) Resume() error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	_, err := s.session.Call("Debugger.resume", nil)
	return err
}

// Pause pauses execution
func (s *StepController) Pause() error {
	if s.session.state.GetState() != StateRunning {
		return fmt.Errorf("debugger is not running")
	}

	_, err := s.session.Call("Debugger.pause", nil)
	return err
}

// StepOver steps over the current statement (F10)
func (s *StepController) StepOver() error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	_, err := s.session.Call("Debugger.stepOver", nil)
	return err
}

// StepInto steps into the current function (F11)
func (s *StepController) StepInto() error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	_, err := s.session.Call("Debugger.stepInto", nil)
	return err
}

// StepOut steps out of the current function (Shift+F11)
func (s *StepController) StepOut() error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	_, err := s.session.Call("Debugger.stepOut", nil)
	return err
}

// StepBack steps back (if time-travel debugging is supported)
func (s *StepController) StepBack() error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	// Note: This may not be supported by all debuggers
	_, err := s.session.Call("Debugger.stepBack", nil)
	return err
}

// ContinueToLocation continues execution until reaching a specific location
func (s *StepController) ContinueToLocation(scriptID string, lineNumber int, column int) error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	params := map[string]any{
		"location": map[string]any{
			"scriptId":   scriptID,
			"lineNumber": lineNumber,
		},
	}

	if column > 0 {
		params["location"].(map[string]any)["columnNumber"] = column
	}

	_, err := s.session.Call("Debugger.continueToLocation", params)
	return err
}

// RestartFrame restarts the current call frame (if supported)
func (s *StepController) RestartFrame(callFrameID string) error {
	if s.session.state.GetState() != StatePaused {
		return fmt.Errorf("debugger is not paused")
	}

	params := map[string]any{
		"callFrameId": callFrameID,
	}

	_, err := s.session.Call("Debugger.restartFrame", params)
	return err
}

// SetSkipAllPauses sets whether to skip all breakpoints temporarily
func (s *StepController) SetSkipAllPauses(skip bool) error {
	params := map[string]any{
		"skip": skip,
	}

	_, err := s.session.Call("Debugger.setSkipAllPauses", params)
	return err
}

// GetCallStack returns the current call stack
func (s *StepController) GetCallStack() []CallFrame {
	return s.session.state.GetCallStack()
}

// GetCurrentFrame returns the topmost call frame
func (s *StepController) GetCurrentFrame() *CallFrame {
	stack := s.session.state.GetCallStack()
	if len(stack) == 0 {
		return nil
	}
	return &stack[0]
}

// GetPauseReason returns why execution was paused
func (s *StepController) GetPauseReason() PauseReason {
	s.session.state.mu.RLock()
	defer s.session.state.mu.RUnlock()
	return s.session.state.PauseReason
}

// IsPaused returns true if the debugger is currently paused
func (s *StepController) IsPaused() bool {
	return s.session.state.GetState() == StatePaused
}

// IsRunning returns true if the debugger is currently running
func (s *StepController) IsRunning() bool {
	return s.session.state.GetState() == StateRunning
}
