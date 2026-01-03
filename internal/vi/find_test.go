package vi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCharFinder_FindChar(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		char           rune
		count          int
		expectedCursor int
		shouldMove     bool
	}{
		{"find first occurrence", "hello", 0, 'l', 1, 2, true},
		{"find second occurrence", "hello", 0, 'l', 2, 3, true},
		{"find from middle", "hello", 1, 'o', 1, 4, true},
		{"not found", "hello", 0, 'x', 1, 0, false},
		{"count exceeds occurrences", "hello", 0, 'l', 5, 0, false},
		{"at end of buffer", "hello", 4, 'o', 1, 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			f := NewCharFinder(b)

			result := f.FindChar(tt.char, tt.count)
			if tt.shouldMove {
				assert.Equal(t, tt.expectedCursor, b.Cursor())
				assert.True(t, result.Inclusive)
			} else {
				assert.Equal(t, tt.startCursor, b.Cursor())
				assert.Equal(t, result.Start, result.End)
			}
		})
	}
}

func TestCharFinder_FindCharBack(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		char           rune
		count          int
		expectedCursor int
		shouldMove     bool
	}{
		{"find backward", "hello", 4, 'l', 1, 3, true},
		{"find second backward", "hello", 4, 'l', 2, 2, true},
		{"not found backward", "hello", 4, 'x', 1, 4, false},
		{"at start of buffer", "hello", 0, 'h', 1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			f := NewCharFinder(b)

			result := f.FindCharBack(tt.char, tt.count)
			if tt.shouldMove {
				assert.Equal(t, tt.expectedCursor, b.Cursor())
			} else {
				assert.Equal(t, tt.startCursor, b.Cursor())
				assert.Equal(t, result.Start, result.End)
			}
		})
	}
}

func TestCharFinder_TillChar(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		char           rune
		count          int
		expectedCursor int
		shouldMove     bool
	}{
		{"till character", "hello", 0, 'l', 1, 1, true},
		{"till second occurrence", "hello", 0, 'l', 2, 2, true},
		{"not found", "hello", 0, 'x', 1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			f := NewCharFinder(b)

			result := f.TillChar(tt.char, tt.count)
			if tt.shouldMove {
				assert.Equal(t, tt.expectedCursor, b.Cursor())
			} else {
				assert.Equal(t, tt.startCursor, b.Cursor())
				assert.Equal(t, result.Start, result.End)
			}
		})
	}
}

func TestCharFinder_TillCharBack(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		startCursor    int
		char           rune
		count          int
		expectedCursor int
		shouldMove     bool
	}{
		{"till backward", "hello", 4, 'l', 1, 4, true},
		{"till second backward", "hello", 4, 'l', 2, 3, true},
		{"not found backward", "hello", 4, 'x', 1, 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer()
			b.SetText(tt.text)
			b.SetCursor(tt.startCursor)
			f := NewCharFinder(b)

			result := f.TillCharBack(tt.char, tt.count)
			if tt.shouldMove {
				assert.Equal(t, tt.expectedCursor, b.Cursor())
			} else {
				assert.Equal(t, tt.startCursor, b.Cursor())
				assert.Equal(t, result.Start, result.End)
			}
		})
	}
}

func TestCharFinder_RepeatFind(t *testing.T) {
	b := NewBuffer()
	b.SetText("abcabc")
	f := NewCharFinder(b)

	// Initial find
	f.FindChar('c', 1)
	assert.Equal(t, 2, b.Cursor())

	// Repeat
	result := f.RepeatFind(1)
	assert.Equal(t, 5, b.Cursor())
	assert.True(t, result.Inclusive)
}

func TestCharFinder_RepeatFindReverse(t *testing.T) {
	b := NewBuffer()
	b.SetText("abcabc")
	f := NewCharFinder(b)

	// Initial find
	f.FindChar('c', 1)
	assert.Equal(t, 2, b.Cursor())

	// Move forward
	b.SetCursor(5)

	// Repeat in reverse direction
	result := f.RepeatFindReverse(1)
	assert.Equal(t, 2, b.Cursor())
	assert.True(t, result.Inclusive)
}

func TestCharFinder_RepeatWithNoLastFind(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello")
	b.SetCursor(2)
	f := NewCharFinder(b)

	// Repeat without previous find should not move
	result := f.RepeatFind(1)
	assert.Equal(t, 2, b.Cursor())
	assert.Equal(t, result.Start, result.End)

	result = f.RepeatFindReverse(1)
	assert.Equal(t, 2, b.Cursor())
	assert.Equal(t, result.Start, result.End)
}

func TestCharFinder_HasLastFind(t *testing.T) {
	b := NewBuffer()
	b.SetText("hello")
	f := NewCharFinder(b)

	assert.False(t, f.HasLastFind())

	f.FindChar('l', 1)
	assert.True(t, f.HasLastFind())
}

func TestCharFinder_TillPreservesType(t *testing.T) {
	b := NewBuffer()
	b.SetText("abcabc")
	f := NewCharFinder(b)

	// Use till
	f.TillChar('c', 1)
	assert.Equal(t, 1, b.Cursor()) // One before 'c'

	// Repeat should also be till
	b.SetCursor(3)
	f.RepeatFind(1)
	assert.Equal(t, 4, b.Cursor()) // One before second 'c'
}
