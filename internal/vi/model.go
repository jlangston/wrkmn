package vi

import (
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubble Tea model for VI mode input
type Model struct {
	buffer    *Buffer
	motions   *Motions
	finder    *CharFinder
	operators *Operators
	history   *History
	search    *Search
	register  *Register
	state     *InputState
	mode      Mode

	// Configuration
	width       int
	placeholder string
	prompt      string

	// onYank, if set, is called with the yanked text after a yank operation
	// (Y, yy, y{motion}). Used by the UI to copy to the system clipboard;
	// kept as a callback so this package stays free of clipboard concerns.
	onYank func(string)
}

// NewModel creates a new VI mode input model
func NewModel() *Model {
	buf := NewBuffer()
	motions := NewMotions(buf)
	finder := NewCharFinder(buf)
	register := &Register{}
	operators := NewOperators(buf, motions, finder, register)
	history := NewHistory(1000)
	search := NewSearch(buf, history)
	state := &InputState{}

	return &Model{
		buffer:    buf,
		motions:   motions,
		finder:    finder,
		operators: operators,
		history:   history,
		search:    search,
		register:  register,
		state:     state,
		mode:      ModeNormal,
		prompt:    "> ",
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return nil
}

// SetPrompt sets the input prompt
func (m *Model) SetPrompt(p string) {
	m.prompt = p
}

// SetOnYank registers a callback invoked with the yanked text after a yank
// (Y, yy, y{motion}). Pass nil to disable. Delete/change do not trigger it.
func (m *Model) SetOnYank(fn func(string)) {
	m.onYank = fn
}

// fireYank invokes the onYank callback if one is registered.
func (m *Model) fireYank(text string) {
	if m.onYank != nil {
		m.onYank(text)
	}
}

// SetPlaceholder sets the placeholder text
func (m *Model) SetPlaceholder(p string) {
	m.placeholder = p
}

// SetWidth sets the input width
func (m *Model) SetWidth(w int) {
	m.width = w
}

// Value returns the current input value
func (m *Model) Value() string {
	return m.buffer.Text()
}

// SetValue sets the input value
func (m *Model) SetValue(s string) {
	m.buffer.SetText(s)
	m.buffer.SetCursor(len([]rune(s)))
}

// Mode returns the current VI mode
func (m *Model) Mode() Mode {
	return m.mode
}

// Cursor returns the cursor position
func (m *Model) Cursor() int {
	return m.buffer.Cursor()
}

// Reset clears the input and history position
func (m *Model) Reset() {
	m.buffer.Clear()
	m.history.Reset()
	m.state.Reset()
	m.mode = ModeNormal
}

// AddHistory adds the current value to history
func (m *Model) AddHistory(entry string) {
	m.history.Add(entry)
}

// Update handles key messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	result := m.HandleKey(keyMsg)

	switch result.Action {
	case ActionSubmit:
		return m, func() tea.Msg { return SubmitMsg{Value: m.Value()} }
	case ActionCancel:
		return m, func() tea.Msg { return CancelMsg{} }
	case ActionHistoryPrev:
		if entry, ok := m.history.Prev(m.Value()); ok {
			m.SetValue(entry)
		}
	case ActionHistoryNext:
		entry, _ := m.history.Next()
		m.SetValue(entry)
	}

	return m, cmd
}

// HandleKey processes a key press and returns the result
func (m *Model) HandleKey(key tea.KeyMsg) KeyResult {
	// Handle search mode first
	if m.search.IsActive() {
		return m.handleSearchKey(key)
	}

	// Handle waiting for character (f, t, F, T, r)
	if m.state.WaitingForChar {
		return m.handleCharWait(key)
	}

	switch m.mode {
	case ModeNormal:
		return m.handleNormalMode(key)
	case ModeInsert:
		return m.handleInsertMode(key)
	case ModeReplace:
		return m.handleReplaceMode(key)
	}

	return KeyResult{Action: ActionNone}
}

// handleNormalMode processes keys in normal mode
func (m *Model) handleNormalMode(key tea.KeyMsg) KeyResult {
	// Handle counts
	if len(key.Runes) == 1 {
		r := key.Runes[0]
		if r >= '1' && r <= '9' || (r == '0' && m.state.Count > 0) {
			m.state.Count = m.state.Count*10 + int(r-'0')
			return KeyResult{Action: ActionHandled}
		}
	}

	count := m.state.GetCount()

	// Handle operators
	if m.state.HasOperator() {
		return m.handleOperatorPending(key, count)
	}

	switch key.Type {
	case tea.KeyEnter:
		m.state.Reset()
		return KeyResult{Action: ActionSubmit}

	case tea.KeyEscape:
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	case tea.KeyCtrlC:
		return KeyResult{Action: ActionCancel}

	case tea.KeyUp:
		m.state.Reset()
		return KeyResult{Action: ActionHistoryPrev}

	case tea.KeyDown:
		m.state.Reset()
		return KeyResult{Action: ActionHistoryNext}
	}

	if len(key.Runes) != 1 {
		return KeyResult{Action: ActionNone}
	}

	r := key.Runes[0]

	switch r {
	// Mode switching
	case 'i':
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'I':
		m.motions.FirstNonBlank()
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'a':
		if m.buffer.Len() > 0 {
			m.buffer.SetCursor(m.buffer.Cursor() + 1)
		}
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'A':
		m.buffer.SetCursor(m.buffer.Len())
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'R':
		m.mode = ModeReplace
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Movement
	case 'h':
		m.motions.Left(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'l':
		m.motions.Right(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'j':
		m.state.Reset()
		return KeyResult{Action: ActionHistoryNext}
	case 'k':
		m.state.Reset()
		return KeyResult{Action: ActionHistoryPrev}
	case '0':
		m.motions.LineStart()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case '^':
		m.motions.FirstNonBlank()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case '$':
		m.motions.LineEnd()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'w':
		m.motions.WordForward(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'W':
		m.motions.WORDForward(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'b':
		m.motions.WordBackward(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'B':
		m.motions.WORDBackward(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'e':
		m.motions.WordEnd(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'E':
		m.motions.WORDEnd(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case '%':
		m.motions.MatchBracket()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Find characters
	case 'f', 'F', 't', 'T':
		m.state.WaitingForChar = true
		m.state.CharCommand = r
		return KeyResult{Action: ActionHandled}
	case ';':
		m.finder.RepeatFind(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case ',':
		m.finder.RepeatFindReverse(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Operators
	case 'd':
		m.state.Operator = OpDelete
		return KeyResult{Action: ActionHandled}
	case 'c':
		m.state.Operator = OpChange
		return KeyResult{Action: ActionHandled}
	case 'y':
		m.state.Operator = OpYank
		return KeyResult{Action: ActionHandled}

	// Operator shortcuts
	case 'x':
		m.operators.DeleteChar(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'X':
		m.operators.DeleteCharBefore(count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 's':
		m.operators.Substitute(count)
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'D':
		m.operators.Delete(m.motions.LineEnd())
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'C':
		m.operators.ChangeToEnd()
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'S':
		m.operators.ChangeLine()
		m.mode = ModeInsert
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'Y':
		m.fireYank(m.operators.YankLine())
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Paste
	case 'p':
		m.operators.Paste()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'P':
		m.operators.PasteBefore()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Replace single char
	case 'r':
		m.state.WaitingForChar = true
		m.state.CharCommand = 'r'
		return KeyResult{Action: ActionHandled}

	// Undo/Redo
	case 'u':
		m.buffer.Undo()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'U':
		// Undo all changes on line (simplified: just undo all)
		for m.buffer.Undo() {
		}
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Case toggle
	case '~':
		for i := 0; i < count; i++ {
			m.buffer.ToggleCase()
		}
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	// Search
	case '/':
		m.search.StartForward()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case '?':
		m.search.StartBackward()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'n':
		m.search.Next()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	case 'N':
		m.search.Prev()
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	}

	m.state.Reset()
	return KeyResult{Action: ActionNone}
}

// handleOperatorPending handles keys when an operator is pending
func (m *Model) handleOperatorPending(key tea.KeyMsg, count int) KeyResult {
	op := m.state.Operator

	// Handle doubled operator (dd, cc, yy)
	if len(key.Runes) == 1 {
		r := key.Runes[0]
		switch {
		case op == OpDelete && r == 'd':
			m.operators.DeleteLine()
			m.state.Reset()
			return KeyResult{Action: ActionHandled}
		case op == OpChange && r == 'c':
			m.operators.ChangeLine()
			m.mode = ModeInsert
			m.state.Reset()
			return KeyResult{Action: ActionHandled}
		case op == OpYank && r == 'y':
			m.fireYank(m.operators.YankLine())
			m.state.Reset()
			return KeyResult{Action: ActionHandled}
		}
	}

	// Get motion for operator
	motion, enterInsert := m.getMotionForOperator(key, count)
	if motion.Start == motion.End && !motion.Inclusive {
		// No motion executed
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	}

	yankedOrDeleted, needInsert := m.operators.ApplyOperator(op, motion)
	if op == OpYank {
		m.fireYank(yankedOrDeleted)
	}
	m.state.Reset()

	if needInsert || enterInsert {
		m.mode = ModeInsert
	}

	return KeyResult{Action: ActionHandled}
}

// getMotionForOperator returns a motion based on key input
func (m *Model) getMotionForOperator(key tea.KeyMsg, count int) (MotionResult, bool) {
	if len(key.Runes) != 1 {
		return MotionResult{}, false
	}

	r := key.Runes[0]

	switch r {
	case 'h':
		return m.motions.Left(count), false
	case 'l':
		return m.motions.Right(count), false
	case '0':
		return m.motions.LineStart(), false
	case '^':
		return m.motions.FirstNonBlank(), false
	case '$':
		return m.motions.LineEnd(), false
	case 'w':
		return m.motions.WordForward(count), false
	case 'W':
		return m.motions.WORDForward(count), false
	case 'b':
		return m.motions.WordBackward(count), false
	case 'B':
		return m.motions.WORDBackward(count), false
	case 'e':
		return m.motions.WordEnd(count), false
	case 'E':
		return m.motions.WORDEnd(count), false
	case '%':
		return m.motions.MatchBracket(), false
	case 'i':
		// Inner text object - wait for next char
		// Simplified: just inner word for now
		return m.motions.InnerWord(), false
	case 'a':
		// A text object - wait for next char
		return m.motions.AWord(), false
	case 'f', 't', 'F', 'T':
		m.state.WaitingForChar = true
		m.state.CharCommand = r
		return MotionResult{}, false
	}

	return MotionResult{}, false
}

// handleCharWait handles the character input after f/t/F/T/r
func (m *Model) handleCharWait(key tea.KeyMsg) KeyResult {
	if key.Type == tea.KeyEscape {
		m.state.WaitingForChar = false
		m.state.CharCommand = 0
		return KeyResult{Action: ActionHandled}
	}

	if len(key.Runes) != 1 {
		return KeyResult{Action: ActionNone}
	}

	char := key.Runes[0]
	count := m.state.GetCount()
	cmd := m.state.CharCommand
	op := m.state.Operator

	m.state.WaitingForChar = false
	m.state.CharCommand = 0

	var motion MotionResult

	switch cmd {
	case 'f':
		motion = m.finder.FindChar(char, count)
	case 'F':
		motion = m.finder.FindCharBack(char, count)
	case 't':
		motion = m.finder.TillChar(char, count)
	case 'T':
		motion = m.finder.TillCharBack(char, count)
	case 'r':
		m.operators.ReplaceChar(char, count)
		m.state.Reset()
		return KeyResult{Action: ActionHandled}
	}

	// Apply operator if pending
	if op != OpNone {
		_, needInsert := m.operators.ApplyOperator(op, motion)
		if needInsert {
			m.mode = ModeInsert
		}
	}

	m.state.Reset()
	return KeyResult{Action: ActionHandled}
}

// handleInsertMode processes keys in insert mode
func (m *Model) handleInsertMode(key tea.KeyMsg) KeyResult {
	switch key.Type {
	case tea.KeyEscape:
		// Move cursor back when leaving insert mode
		if m.buffer.Cursor() > 0 {
			m.buffer.SetCursor(m.buffer.Cursor() - 1)
		}
		m.mode = ModeNormal
		m.state.Reset()
		return KeyResult{Action: ActionHandled}

	case tea.KeyEnter:
		return KeyResult{Action: ActionSubmit}

	case tea.KeyCtrlC:
		return KeyResult{Action: ActionCancel}

	case tea.KeyBackspace:
		m.buffer.Backspace()
		return KeyResult{Action: ActionHandled}

	case tea.KeyCtrlH:
		m.buffer.Backspace()
		return KeyResult{Action: ActionHandled}

	case tea.KeyCtrlW:
		m.buffer.DeleteWord()
		return KeyResult{Action: ActionHandled}

	case tea.KeyCtrlU:
		m.buffer.DeleteToStart()
		return KeyResult{Action: ActionHandled}

	case tea.KeyLeft:
		m.motions.Left(1)
		return KeyResult{Action: ActionHandled}

	case tea.KeyRight:
		m.motions.Right(1)
		return KeyResult{Action: ActionHandled}

	case tea.KeyUp:
		return KeyResult{Action: ActionHistoryPrev}

	case tea.KeyDown:
		return KeyResult{Action: ActionHistoryNext}

	case tea.KeySpace:
		m.buffer.Insert(' ')
		return KeyResult{Action: ActionHandled}

	default:
		if len(key.Runes) > 0 && unicode.IsPrint(key.Runes[0]) {
			m.buffer.Insert(key.Runes[0])
			return KeyResult{Action: ActionHandled}
		}
	}

	return KeyResult{Action: ActionNone}
}

// handleReplaceMode processes keys in replace mode
func (m *Model) handleReplaceMode(key tea.KeyMsg) KeyResult {
	switch key.Type {
	case tea.KeyEscape:
		if m.buffer.Cursor() > 0 {
			m.buffer.SetCursor(m.buffer.Cursor() - 1)
		}
		m.mode = ModeNormal
		return KeyResult{Action: ActionHandled}

	case tea.KeyEnter:
		return KeyResult{Action: ActionSubmit}

	case tea.KeyCtrlC:
		return KeyResult{Action: ActionCancel}

	case tea.KeyBackspace:
		// In replace mode, backspace moves back but doesn't delete
		m.motions.Left(1)
		return KeyResult{Action: ActionHandled}

	default:
		if len(key.Runes) > 0 && unicode.IsPrint(key.Runes[0]) {
			m.buffer.Replace(key.Runes[0])
			return KeyResult{Action: ActionHandled}
		}
	}

	return KeyResult{Action: ActionNone}
}

// handleSearchKey handles keys during search input
func (m *Model) handleSearchKey(key tea.KeyMsg) KeyResult {
	switch key.Type {
	case tea.KeyEscape:
		m.search.Cancel()
		return KeyResult{Action: ActionHandled}

	case tea.KeyEnter:
		m.search.Execute()
		return KeyResult{Action: ActionHandled}

	case tea.KeyBackspace, tea.KeyCtrlH:
		if !m.search.RemoveChar() {
			m.search.Cancel()
		}
		return KeyResult{Action: ActionHandled}

	default:
		if len(key.Runes) > 0 && unicode.IsPrint(key.Runes[0]) {
			m.search.AddChar(key.Runes[0])
			return KeyResult{Action: ActionHandled}
		}
	}

	return KeyResult{Action: ActionNone}
}

// View renders the input
func (m *Model) View() string {
	// This is a simplified view - the full UI will style this appropriately
	text := m.buffer.Text()
	if text == "" && m.placeholder != "" && m.mode == ModeNormal {
		return m.prompt + m.placeholder
	}

	// Render with cursor indicator
	if m.search.IsActive() {
		return m.search.GetDirection() + m.search.GetInputBuffer() + "_"
	}

	return m.prompt + text
}

// SubmitMsg is sent when the user submits input
type SubmitMsg struct {
	Value string
}

// CancelMsg is sent when the user cancels input
type CancelMsg struct{}
