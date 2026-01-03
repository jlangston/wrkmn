package vi

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mode represents the current VI mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeReplace
)

func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeReplace:
		return "REPLACE"
	default:
		return "UNKNOWN"
	}
}

// Operator represents a VI operator (d, c, y)
type Operator int

const (
	OpNone Operator = iota
	OpDelete
	OpChange
	OpYank
)

func (o Operator) String() string {
	switch o {
	case OpDelete:
		return "d"
	case OpChange:
		return "c"
	case OpYank:
		return "y"
	default:
		return ""
	}
}

// Motion represents a VI motion
type Motion int

const (
	MotionNone Motion = iota
	// Character motions
	MotionLeft          // h
	MotionRight         // l
	MotionDown          // j (in multi-line)
	MotionUp            // k (in multi-line)
	// Word motions
	MotionWordForward   // w
	MotionWordBackward  // b
	MotionWordEnd       // e
	MotionWORDForward   // W
	MotionWORDBackward  // B
	MotionWORDEnd       // E
	// Line motions
	MotionLineStart     // 0
	MotionFirstNonBlank // ^
	MotionLineEnd       // $
	// Find motions
	MotionFindChar      // f<char>
	MotionFindCharBack  // F<char>
	MotionTillChar      // t<char>
	MotionTillCharBack  // T<char>
	// Other
	MotionMatchBracket  // %
	MotionInnerWord     // iw
	MotionAWord         // aw
)

// UndoEntry represents a single undoable change
type UndoEntry struct {
	Text       string // The buffer content before the change
	CursorPos  int    // Cursor position before the change
}

// Cursor represents the cursor state
type Cursor struct {
	Pos int // Position in the buffer
}

// Register holds yanked/deleted text
type Register struct {
	Content string
	Linewise bool
}

// InputState tracks partial input for multi-key commands
type InputState struct {
	Count        int      // Numeric prefix (e.g., 3dw)
	Operator     Operator // Pending operator
	WaitingForChar bool   // Waiting for char (f, t, F, T, r)
	CharCommand  rune     // The command waiting for char (f, t, etc.)
	SearchMode   bool     // Currently entering search pattern
	SearchBuffer string   // Buffer for search input
	SearchDirection int   // 1 = forward, -1 = backward
}

// Reset clears the input state
func (s *InputState) Reset() {
	s.Count = 0
	s.Operator = OpNone
	s.WaitingForChar = false
	s.CharCommand = 0
	s.SearchMode = false
	s.SearchBuffer = ""
}

// HasOperator returns true if an operator is pending
func (s *InputState) HasOperator() bool {
	return s.Operator != OpNone
}

// GetCount returns the count, defaulting to 1
func (s *InputState) GetCount() int {
	if s.Count == 0 {
		return 1
	}
	return s.Count
}

// KeyAction represents the result of processing a key
type KeyAction int

const (
	ActionNone KeyAction = iota
	ActionHandled      // Key was handled, update view
	ActionSubmit       // Submit the current line
	ActionCancel       // Cancel input (Ctrl-C, Esc in insert mode)
	ActionHistoryPrev  // Navigate to previous history
	ActionHistoryNext  // Navigate to next history
)

// KeyResult contains the result of processing a key
type KeyResult struct {
	Action  KeyAction
	Message tea.Msg // Optional message to send
}
