package vi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMotions_Left(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"move left once", "hello", 3, 1, 2},
		{"move left multiple", "hello", 4, 3, 1},
		{"clamp at start", "hello", 1, 5, 0},
		{"already at start", "hello", 0, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.Left(tt.count)
			assert.Equal(t, tt.startCursor, result.Start)
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_Right(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"move right once", "hello", 0, 1, 1},
		{"move right multiple", "hello", 0, 3, 3},
		{"clamp at end", "hello", 3, 5, 4},
		{"already at end", "hello", 4, 1, 4},
		{"empty buffer", "", 0, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.Right(tt.count)
			assert.Equal(t, tt.startCursor, result.Start)
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_LineStart(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello world")
	b.SetCursor(5)
	m := NewMotions(b)

	result := m.LineStart()
	assert.Equal(t, 5, result.Start)
	assert.Equal(t, 0, result.End)
	assert.Equal(t, 0, b.Cursor())
}

func TestMotions_LineEnd(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		expectedCursor int
		expectedEnd    int
	}{
		{"from start", "hello", 0, 4, 5},
		{"from middle", "hello", 2, 4, 5},
		{"already at end", "hello", 4, 4, 5},
		{"empty buffer", "", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.LineEnd()
			assert.Equal(t, tt.startCursor, result.Start)
			assert.Equal(t, tt.expectedEnd, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
			assert.True(t, result.Inclusive)
		})
	}
}

func TestMotions_FirstNonBlank(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		expectedCursor int
	}{
		{"no leading space", "hello", 0},
		{"leading spaces", "   hello", 3},
		{"leading tabs", "\t\thello", 2},
		{"all spaces", "     ", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(5)
			m := NewMotions(b)

			result := m.FirstNonBlank()
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_WordForward(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"simple word", "hello world", 0, 1, 6},
		{"multiple words", "one two three", 0, 2, 8},
		{"from middle of word", "hello world", 2, 1, 6},
		{"at end", "hello world", 10, 1, 10},
		{"with punctuation", "hello.world test", 0, 1, 6}, // jumps to 'w' after punctuation
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.WordForward(tt.count)
			assert.Equal(t, tt.startCursor, result.Start)
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_WORDForward(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"with punctuation", "hello.world test", 0, 1, 12},
		{"simple", "hello world", 0, 1, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.WORDForward(tt.count)
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_WordBackward(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"simple word", "hello world", 7, 1, 6},
		{"multiple words", "one two three", 12, 2, 4},
		{"at start", "hello world", 0, 1, 0},
		{"from start of word", "hello world", 6, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.WordBackward(tt.count)
			assert.Equal(t, tt.startCursor, result.Start)
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_WORDBackward(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"with punctuation", "hello.world test", 15, 1, 12},
		{"skip punctuation", "hello.world test", 12, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.WORDBackward(tt.count)
			assert.Equal(t, tt.expectedCursor, result.End)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
		})
	}
}

func TestMotions_WordEnd(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		count          int
		expectedCursor int
	}{
		{"end of first word", "hello world", 0, 1, 4},
		{"end of second word", "hello world", 0, 2, 10},
		{"from middle", "hello world", 2, 1, 4},
		{"at end", "hello", 4, 1, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.WordEnd(tt.count)
			assert.Equal(t, tt.expectedCursor, b.Cursor())
			assert.True(t, result.Inclusive)
		})
	}
}

func TestMotions_MatchBracket(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		expectedCursor int
		shouldMove     bool
	}{
		{"parens forward", "(hello)", 0, 6, true},
		{"parens backward", "(hello)", 6, 0, true},
		{"nested parens", "((a))", 0, 4, true},
		{"brackets", "[test]", 0, 5, true},
		{"braces", "{code}", 0, 5, true},
		{"no bracket", "hello", 0, 0, false},
		{"unmatched", "(hello", 0, 0, false},
		{"find bracket ahead", "xx(yy)", 0, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			m := NewMotions(b)

			result := m.MatchBracket()
			if tt.shouldMove {
				assert.Equal(t, tt.expectedCursor, b.Cursor())
				assert.Equal(t, tt.expectedCursor, result.End)
				assert.True(t, result.Inclusive)
			} else {
				assert.Equal(t, tt.startCursor, b.Cursor())
				assert.Equal(t, result.Start, result.End)
			}
		})
	}
}

func TestMotions_InnerWord(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		cursor        int
		expectedStart int
		expectedEnd   int
	}{
		{"on word", "hello world", 7, 6, 11},
		{"on first word", "hello world", 2, 0, 5},
		{"on whitespace", "hello  world", 6, 5, 7},
		{"single char word", "a b c", 2, 2, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.cursor)
			m := NewMotions(b)

			result := m.InnerWord()
			assert.Equal(t, tt.expectedStart, result.Start)
			assert.Equal(t, tt.expectedEnd, result.End)
			assert.True(t, result.Inclusive)
		})
	}
}

func TestMotions_AWord(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		cursor        int
		expectedStart int
		expectedEnd   int
	}{
		{"word with trailing space", "hello world test", 6, 6, 12},
		{"last word with leading space", "hello world", 6, 5, 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.cursor)
			m := NewMotions(b)

			result := m.AWord()
			assert.Equal(t, tt.expectedStart, result.Start)
			assert.Equal(t, tt.expectedEnd, result.End)
		})
	}
}
