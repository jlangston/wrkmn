package vi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuffer(t *testing.T) {
	b := NewBuffer()
	assert.Equal(t, "", b.Text())
	assert.Equal(t, 0, b.Cursor())
	assert.Equal(t, 0, b.Len())
}

func TestBuffer_SetText(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		initialCursor  int
		newText        string
		expectedCursor int
	}{
		{"set empty buffer", "", 0, "hello", 0},
		{"replace text", "hello", 2, "world", 2},
		{"cursor clamped when text shorter", "hello world", 10, "hi", 2},
		{"cursor stays at end", "hello", 5, "hi", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.initialCursor)
			b.SetText(tt.newText)
			assert.Equal(t, tt.newText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_SetCursor(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		pos      int
		expected int
	}{
		{"valid position", "hello", 2, 2},
		{"negative clamped to 0", "hello", -5, 0},
		{"past end clamped", "hello", 10, 5},
		{"at end", "hello", 5, 5},
		{"empty buffer", "", 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.pos)
			assert.Equal(t, tt.expected, b.Cursor())
		})
	}
}

func TestBuffer_RuneAt(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello")

	tests := []struct {
		pos      int
		expected rune
	}{
		{0, 'h'},
		{4, 'o'},
		{-1, 0},
		{5, 0},
		{100, 0},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, b.RuneAt(tt.pos), "RuneAt(%d)", tt.pos)
	}
}

func TestBuffer_Insert(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		insertRune     rune
		expectedText   string
		expectedCursor int
	}{
		{"insert at start", "hello", 0, 'X', "Xhello", 1},
		{"insert in middle", "hello", 2, 'X', "heXllo", 3},
		{"insert at end", "hello", 5, 'X', "helloX", 6},
		{"insert into empty", "", 0, 'X', "X", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			b.Insert(tt.insertRune)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_InsertString(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		insertStr      string
		expectedText   string
		expectedCursor int
	}{
		{"insert at start", "world", 0, "hello ", "hello world", 6},
		{"insert in middle", "helo", 2, "ll", "hello", 4},
		{"insert at end", "hello", 5, " world", "hello world", 11},
		{"insert empty string", "hello", 2, "", "hello", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			b.InsertString(tt.insertStr)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_Replace(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		replaceRune    rune
		expectedText   string
		expectedCursor int
	}{
		{"replace at start", "hello", 0, 'X', "Xello", 1},
		{"replace in middle", "hello", 2, 'X', "heXlo", 3},
		{"replace at end", "hello", 4, 'X', "hellX", 5},
		{"replace past end inserts", "hello", 5, 'X', "helloX", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			b.Replace(tt.replaceRune)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_DeleteChar(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		expectedRune   rune
		expectedText   string
		expectedCursor int
	}{
		{"delete at start", "hello", 0, 'h', "ello", 0},
		{"delete in middle", "hello", 2, 'l', "helo", 2},
		{"delete at end", "hello", 4, 'o', "hell", 3},
		{"delete past end", "hello", 5, 0, "hello", 5},
		{"delete last char", "x", 0, 'x', "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			r := b.DeleteChar()
			assert.Equal(t, tt.expectedRune, r)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_DeleteCharBefore(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		expectedRune   rune
		expectedText   string
		expectedCursor int
	}{
		{"delete before start", "hello", 0, 0, "hello", 0},
		{"delete in middle", "hello", 2, 'e', "hllo", 1},
		{"delete at end", "hello", 5, 'o', "hell", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			r := b.DeleteCharBefore()
			assert.Equal(t, tt.expectedRune, r)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_Backspace(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		expectedOK     bool
		expectedText   string
		expectedCursor int
	}{
		{"backspace at start", "hello", 0, false, "hello", 0},
		{"backspace in middle", "hello", 2, true, "hllo", 1},
		{"backspace at end", "hello", 5, true, "hell", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			ok := b.Backspace()
			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_DeleteRange(t *testing.T) {
	tests := []struct {
		name            string
		initial         string
		start           int
		end             int
		expectedDeleted string
		expectedText    string
		expectedCursor  int
	}{
		{"delete middle", "hello world", 5, 11, " world", "hello", 4},
		{"delete start", "hello world", 0, 6, "hello ", "world", 0},
		{"reversed range", "hello", 4, 1, "ell", "ho", 1},
		{"same position", "hello", 2, 2, "", "hello", 2},
		{"clamped range", "hello", -5, 100, "hello", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			deleted := b.DeleteRange(tt.start, tt.end)
			assert.Equal(t, tt.expectedDeleted, deleted)
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_DeleteToEnd(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello world")
	b.SetCursor(5)
	deleted := b.DeleteToEnd()
	assert.Equal(t, " world", deleted)
	assert.Equal(t, "hello", b.Text())
	assert.Equal(t, 4, b.Cursor())
}

func TestBuffer_DeleteWord(t *testing.T) {
	tests := []struct {
		name            string
		initial         string
		cursorPos       int
		expectedDeleted string
		expectedText    string
	}{
		{"delete word", "hello world", 11, "world", "hello "},
		{"delete with trailing space", "hello  world", 7, "  ", "helloworld"},
		{"at start", "hello", 0, "", "hello"},
		{"single word", "hello", 5, "hello", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			deleted := b.DeleteWord()
			assert.Equal(t, tt.expectedDeleted, deleted)
			assert.Equal(t, tt.expectedText, b.Text())
		})
	}
}

func TestBuffer_DeleteToStart(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello world")
	b.SetCursor(6)
	deleted := b.DeleteToStart()
	assert.Equal(t, "hello ", deleted)
	assert.Equal(t, "world", b.Text())
	assert.Equal(t, 0, b.Cursor())
}

func TestBuffer_Clear(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello world")
	b.SetCursor(5)
	b.Clear()
	assert.Equal(t, "", b.Text())
	assert.Equal(t, 0, b.Cursor())
}

func TestBuffer_ToggleCase(t *testing.T) {
	tests := []struct {
		name           string
		initial        string
		cursorPos      int
		expectedText   string
		expectedCursor int
	}{
		{"toggle lowercase", "hello", 0, "Hello", 1},
		{"toggle uppercase", "HELLO", 0, "hELLO", 1},
		{"non-letter unchanged", "h3llo", 1, "h3llo", 2},
		{"at end stays", "hellO", 4, "hello", 4},
		{"past end no-op", "hello", 5, "hello", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.initial)
			b.SetCursor(tt.cursorPos)
			b.ToggleCase()
			assert.Equal(t, tt.expectedText, b.Text())
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestBuffer_Undo(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello")

	// Make a change
	b.Insert('X')
	require.Equal(t, "Xhello", b.Text())

	// Undo should restore
	ok := b.Undo()
	assert.True(t, ok)
	assert.Equal(t, "hello", b.Text())

	// Undo on empty stack returns false
	ok = b.Undo()
	assert.False(t, ok)
}

func TestBuffer_Redo(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello")

	// Make a change
	b.Insert('X')
	require.Equal(t, "Xhello", b.Text())

	// Undo
	b.Undo()
	require.Equal(t, "hello", b.Text())

	// Redo should restore the change
	ok := b.Redo()
	assert.True(t, ok)
	assert.Equal(t, "Xhello", b.Text())

	// Redo on empty stack returns false
	ok = b.Redo()
	assert.False(t, ok)
}

func TestBuffer_UndoRedoChain(t *testing.T) {
	b := NewBuffer()

	// Make multiple changes
	b.Insert('a')
	b.Insert('b')
	b.Insert('c')
	require.Equal(t, "abc", b.Text())

	// Undo all
	b.Undo()
	assert.Equal(t, "ab", b.Text())
	b.Undo()
	assert.Equal(t, "a", b.Text())
	b.Undo()
	assert.Equal(t, "", b.Text())

	// Redo all
	b.Redo()
	assert.Equal(t, "a", b.Text())
	b.Redo()
	assert.Equal(t, "ab", b.Text())
	b.Redo()
	assert.Equal(t, "abc", b.Text())
}

func TestBuffer_NewChangeClearsRedoStack(t *testing.T) {
	b := NewBuffer()

	b.Insert('a')
	b.Insert('b')
	b.Undo() // Now at "a", redo stack has "ab"

	// New change should clear redo stack
	b.Insert('X')
	assert.Equal(t, "aX", b.Text())

	// Redo should fail since redo stack was cleared
	ok := b.Redo()
	assert.False(t, ok)
}

func TestBuffer_UndoStackLimit(t *testing.T) {
	b := NewBuffer()

	// Make more changes than max undo (100)
	for i := 0; i < 110; i++ {
		b.Insert('a')
	}

	// Should only be able to undo maxUndo times
	count := 0
	for b.Undo() {
		count++
	}
	assert.Equal(t, 100, count)
}

func TestBuffer_SubstringBetween(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello world")

	tests := []struct {
		start    int
		end      int
		expected string
	}{
		{0, 5, "hello"},
		{6, 11, "world"},
		{5, 0, "hello"},  // reversed
		{-5, 5, "hello"}, // clamped
		{0, 100, "hello world"},
	}

	for _, tt := range tests {
		result := b.SubstringBetween(tt.start, tt.end)
		assert.Equal(t, tt.expected, result, "SubstringBetween(%d, %d)", tt.start, tt.end)
	}
}

func TestBuffer_FindForward(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello hello")

	tests := []struct {
		cursorPos int
		pattern   string
		expected  int
	}{
		{0, "llo", 2},
		{3, "llo", 8},
		{0, "xyz", -1},
		{0, "", -1},
		{10, "o", -1}, // at end
	}

	for _, tt := range tests {
		b.SetCursor(tt.cursorPos)
		result := b.FindForward(tt.pattern)
		assert.Equal(t, tt.expected, result, "FindForward(%q) from %d", tt.pattern, tt.cursorPos)
	}
}

func TestBuffer_FindBackward(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello hello")

	tests := []struct {
		cursorPos int
		pattern   string
		expected  int
	}{
		{11, "llo", 8},
		{7, "llo", 2},
		{11, "xyz", -1},
		{11, "", -1},
		{0, "h", -1}, // at start
	}

	for _, tt := range tests {
		b.SetCursor(tt.cursorPos)
		result := b.FindBackward(tt.pattern)
		assert.Equal(t, tt.expected, result, "FindBackward(%q) from %d", tt.pattern, tt.cursorPos)
	}
}

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		r        rune
		expected bool
	}{
		{'a', true},
		{'Z', true},
		{'5', true},
		{'_', true},
		{' ', false},
		{'.', false},
		{'-', false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, IsWordChar(tt.r), "IsWordChar(%q)", tt.r)
	}
}

func TestIsWORDChar(t *testing.T) {
	tests := []struct {
		r        rune
		expected bool
	}{
		{'a', true},
		{'.', true},
		{'-', true},
		{' ', false},
		{'\t', false},
		{'\n', false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, IsWORDChar(tt.r), "IsWORDChar(%q)", tt.r)
	}
}
