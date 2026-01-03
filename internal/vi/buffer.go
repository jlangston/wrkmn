package vi

import (
	"strings"
	"unicode"
)

// Buffer represents an editable text buffer with undo capability
type Buffer struct {
	text      []rune     // The text content
	cursor    int        // Cursor position
	undoStack []UndoEntry // Undo history
	redoStack []UndoEntry // Redo history
	maxUndo   int         // Maximum undo entries
	lastChange string     // Last change for dot repeat
}

// NewBuffer creates a new empty buffer
func NewBuffer() *Buffer {
	return &Buffer{
		text:      make([]rune, 0),
		cursor:    0,
		undoStack: make([]UndoEntry, 0),
		redoStack: make([]UndoEntry, 0),
		maxUndo:   100,
	}
}

// Text returns the buffer content as a string
func (b *Buffer) Text() string {
	return string(b.text)
}

// SetText sets the buffer content
func (b *Buffer) SetText(s string) {
	b.text = []rune(s)
	b.cursor = min(b.cursor, len(b.text))
}

// Len returns the length of the buffer
func (b *Buffer) Len() int {
	return len(b.text)
}

// Cursor returns the current cursor position
func (b *Buffer) Cursor() int {
	return b.cursor
}

// SetCursor sets the cursor position
func (b *Buffer) SetCursor(pos int) {
	b.cursor = clamp(pos, 0, len(b.text))
}

// RuneAt returns the rune at position, or 0 if out of bounds
func (b *Buffer) RuneAt(pos int) rune {
	if pos < 0 || pos >= len(b.text) {
		return 0
	}
	return b.text[pos]
}

// saveUndo saves current state for undo
func (b *Buffer) saveUndo() {
	entry := UndoEntry{
		Text:      string(b.text),
		CursorPos: b.cursor,
	}
	b.undoStack = append(b.undoStack, entry)
	if len(b.undoStack) > b.maxUndo {
		b.undoStack = b.undoStack[1:]
	}
	// Clear redo stack on new change
	b.redoStack = nil
}

// Undo reverts the last change
func (b *Buffer) Undo() bool {
	if len(b.undoStack) == 0 {
		return false
	}

	// Save current state to redo
	b.redoStack = append(b.redoStack, UndoEntry{
		Text:      string(b.text),
		CursorPos: b.cursor,
	})

	// Pop from undo stack
	entry := b.undoStack[len(b.undoStack)-1]
	b.undoStack = b.undoStack[:len(b.undoStack)-1]

	b.text = []rune(entry.Text)
	b.cursor = entry.CursorPos
	return true
}

// Redo reapplies the last undone change
func (b *Buffer) Redo() bool {
	if len(b.redoStack) == 0 {
		return false
	}

	// Save current state to undo
	b.undoStack = append(b.undoStack, UndoEntry{
		Text:      string(b.text),
		CursorPos: b.cursor,
	})

	// Pop from redo stack
	entry := b.redoStack[len(b.redoStack)-1]
	b.redoStack = b.redoStack[:len(b.redoStack)-1]

	b.text = []rune(entry.Text)
	b.cursor = entry.CursorPos
	return true
}

// Insert inserts a rune at the cursor position
func (b *Buffer) Insert(r rune) {
	b.saveUndo()
	b.text = append(b.text[:b.cursor], append([]rune{r}, b.text[b.cursor:]...)...)
	b.cursor++
}

// InsertString inserts a string at the cursor position
func (b *Buffer) InsertString(s string) {
	b.saveUndo()
	runes := []rune(s)
	b.text = append(b.text[:b.cursor], append(runes, b.text[b.cursor:]...)...)
	b.cursor += len(runes)
}

// Replace replaces the rune at cursor with another
func (b *Buffer) Replace(r rune) {
	if b.cursor >= len(b.text) {
		b.Insert(r)
		return
	}
	b.saveUndo()
	b.text[b.cursor] = r
	b.cursor++
}

// DeleteChar deletes the character at cursor (x command)
func (b *Buffer) DeleteChar() rune {
	if b.cursor >= len(b.text) {
		return 0
	}
	b.saveUndo()
	r := b.text[b.cursor]
	b.text = append(b.text[:b.cursor], b.text[b.cursor+1:]...)
	// Adjust cursor if at end
	if b.cursor >= len(b.text) && b.cursor > 0 {
		b.cursor--
	}
	return r
}

// DeleteCharBefore deletes the character before cursor (X command)
func (b *Buffer) DeleteCharBefore() rune {
	if b.cursor == 0 {
		return 0
	}
	b.saveUndo()
	b.cursor--
	r := b.text[b.cursor]
	b.text = append(b.text[:b.cursor], b.text[b.cursor+1:]...)
	return r
}

// Backspace deletes the character before cursor (insert mode backspace)
func (b *Buffer) Backspace() bool {
	if b.cursor == 0 {
		return false
	}
	b.saveUndo()
	b.cursor--
	b.text = append(b.text[:b.cursor], b.text[b.cursor+1:]...)
	return true
}

// DeleteRange deletes text between start and end positions
func (b *Buffer) DeleteRange(start, end int) string {
	if start > end {
		start, end = end, start
	}
	start = clamp(start, 0, len(b.text))
	end = clamp(end, 0, len(b.text))
	if start == end {
		return ""
	}

	b.saveUndo()
	deleted := string(b.text[start:end])
	b.text = append(b.text[:start], b.text[end:]...)
	b.cursor = start
	if b.cursor >= len(b.text) && b.cursor > 0 {
		b.cursor = len(b.text) - 1
	}
	return deleted
}

// DeleteToEnd deletes from cursor to end of line (D command)
func (b *Buffer) DeleteToEnd() string {
	return b.DeleteRange(b.cursor, len(b.text))
}

// DeleteWord deletes from cursor to start of previous word (Ctrl-W)
func (b *Buffer) DeleteWord() string {
	if b.cursor == 0 {
		return ""
	}

	start := b.cursor - 1

	// Skip trailing whitespace
	for start > 0 && unicode.IsSpace(b.text[start]) {
		start--
	}

	// Find start of word
	for start > 0 && !unicode.IsSpace(b.text[start-1]) {
		start--
	}

	return b.DeleteRange(start, b.cursor)
}

// DeleteToStart deletes from cursor to start of line (Ctrl-U)
func (b *Buffer) DeleteToStart() string {
	return b.DeleteRange(0, b.cursor)
}

// Clear clears the buffer
func (b *Buffer) Clear() {
	b.saveUndo()
	b.text = nil
	b.cursor = 0
}

// ToggleCase toggles the case of the character at cursor (~)
func (b *Buffer) ToggleCase() {
	if b.cursor >= len(b.text) {
		return
	}
	b.saveUndo()
	r := b.text[b.cursor]
	if unicode.IsUpper(r) {
		b.text[b.cursor] = unicode.ToLower(r)
	} else if unicode.IsLower(r) {
		b.text[b.cursor] = unicode.ToUpper(r)
	}
	if b.cursor < len(b.text)-1 {
		b.cursor++
	}
}

// WordBoundary helpers

// IsWordChar returns true if r is a word character (alphanumeric or underscore)
func IsWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// IsWORDChar returns true if r is a WORD character (non-whitespace)
func IsWORDChar(r rune) bool {
	return !unicode.IsSpace(r)
}

// helper functions

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SubstringBetween returns the text between two positions
func (b *Buffer) SubstringBetween(start, end int) string {
	if start > end {
		start, end = end, start
	}
	start = clamp(start, 0, len(b.text))
	end = clamp(end, 0, len(b.text))
	return string(b.text[start:end])
}

// String matching for searching

// FindForward finds the next occurrence of pattern after cursor
func (b *Buffer) FindForward(pattern string) int {
	if pattern == "" || b.cursor >= len(b.text)-1 {
		return -1
	}
	text := string(b.text[b.cursor+1:])
	idx := strings.Index(text, pattern)
	if idx == -1 {
		return -1
	}
	return b.cursor + 1 + idx
}

// FindBackward finds the previous occurrence of pattern before cursor
func (b *Buffer) FindBackward(pattern string) int {
	if pattern == "" || b.cursor == 0 {
		return -1
	}
	text := string(b.text[:b.cursor])
	idx := strings.LastIndex(text, pattern)
	return idx
}
