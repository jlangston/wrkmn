package vi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestOperators() (*Operators, *Buffer, *Register) {
	buf := NewBuffer()
	motions := NewMotions(buf)
	finder := NewCharFinder(buf)
	reg := &Register{}
	ops := NewOperators(buf, motions, finder, reg)
	return ops, buf, reg
}

func TestOperators_Delete(t *testing.T) {
	tests := []struct {
		name            string
		text            string
		motionStart     int
		motionEnd       int
		expectedText    string
		expectedDeleted string
	}{
		{"delete word", "hello world", 0, 5, " world", "hello"},
		{"delete middle", "hello world", 5, 11, "hello", " world"},
		{"delete reversed range", "hello world", 11, 5, "hello", " world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, buf, reg := newTestOperators()
			buf.SetText(tt.text)

			motion := MotionResult{Start: tt.motionStart, End: tt.motionEnd, Inclusive: false}
			deleted := ops.Delete(motion)

			assert.Equal(t, tt.expectedDeleted, deleted)
			assert.Equal(t, tt.expectedText, buf.Text())
			assert.Equal(t, tt.expectedDeleted, reg.Content)
			assert.False(t, reg.Linewise)
		})
	}
}

func TestOperators_DeleteLine(t *testing.T) {
	ops, buf, reg := newTestOperators()
	buf.SetText("hello world")

	deleted := ops.DeleteLine()

	assert.Equal(t, "hello world", deleted)
	assert.Equal(t, "", buf.Text())
	assert.Equal(t, "hello world", reg.Content)
	assert.True(t, reg.Linewise)
}

func TestOperators_Change(t *testing.T) {
	ops, buf, reg := newTestOperators()
	buf.SetText("hello world")

	motion := MotionResult{Start: 0, End: 5, Inclusive: false}
	deleted := ops.Change(motion)

	assert.Equal(t, "hello", deleted)
	assert.Equal(t, " world", buf.Text())
	assert.Equal(t, "hello", reg.Content)
	assert.False(t, reg.Linewise)
}

func TestOperators_ChangeLine(t *testing.T) {
	ops, buf, reg := newTestOperators()
	buf.SetText("hello world")

	deleted := ops.ChangeLine()

	assert.Equal(t, "hello world", deleted)
	assert.Equal(t, "", buf.Text())
	assert.True(t, reg.Linewise)
}

func TestOperators_ChangeToEnd(t *testing.T) {
	ops, buf, _ := newTestOperators()
	buf.SetText("hello world")
	buf.SetCursor(5)

	deleted := ops.ChangeToEnd()

	assert.Equal(t, " world", deleted)
	assert.Equal(t, "hello", buf.Text())
}

func TestOperators_Yank(t *testing.T) {
	ops, buf, reg := newTestOperators()
	buf.SetText("hello world")

	motion := MotionResult{Start: 0, End: 5, Inclusive: false}
	yanked := ops.Yank(motion)

	assert.Equal(t, "hello", yanked)
	assert.Equal(t, "hello world", buf.Text()) // Text unchanged
	assert.Equal(t, "hello", reg.Content)
	assert.False(t, reg.Linewise)
}

func TestOperators_YankLine(t *testing.T) {
	ops, buf, reg := newTestOperators()
	buf.SetText("hello world")

	yanked := ops.YankLine()

	assert.Equal(t, "hello world", yanked)
	assert.Equal(t, "hello world", buf.Text()) // Text unchanged
	assert.Equal(t, "hello world", reg.Content)
	assert.True(t, reg.Linewise)
}

func TestOperators_Paste(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		cursor         int
		pasteContent   string
		expectedText   string
		expectedCursor int
	}{
		{"paste after cursor", "hello", 1, "XX", "heXXllo", 3},
		{"paste at end", "hello", 4, "XX", "helloXX", 6},
		{"empty register", "hello", 2, "", "hello", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, buf, reg := newTestOperators()
			buf.SetText(tt.text)
			buf.SetCursor(tt.cursor)
			reg.Content = tt.pasteContent

			ops.Paste()

			assert.Equal(t, tt.expectedText, buf.Text())
			if tt.pasteContent != "" {
				assert.Equal(t, tt.expectedCursor, buf.Cursor())
			}
		})
	}
}

func TestOperators_PasteBefore(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		cursor         int
		pasteContent   string
		expectedText   string
		expectedCursor int
	}{
		{"paste before cursor", "hello", 2, "XX", "heXXllo", 2},
		{"paste at start", "hello", 0, "XX", "XXhello", 0},
		{"empty register", "hello", 2, "", "hello", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, buf, reg := newTestOperators()
			buf.SetText(tt.text)
			buf.SetCursor(tt.cursor)
			reg.Content = tt.pasteContent

			ops.PasteBefore()

			assert.Equal(t, tt.expectedText, buf.Text())
			if tt.pasteContent != "" {
				assert.Equal(t, tt.expectedCursor, buf.Cursor())
			}
		})
	}
}

func TestOperators_ApplyOperator(t *testing.T) {
	t.Run("delete operator", func(t *testing.T) {
		ops, buf, _ := newTestOperators()
		buf.SetText("hello world")

		motion := MotionResult{Start: 0, End: 5, Inclusive: false}
		result, enterInsert := ops.ApplyOperator(OpDelete, motion)

		assert.Equal(t, "hello", result)
		assert.False(t, enterInsert)
		assert.Equal(t, " world", buf.Text())
	})

	t.Run("change operator", func(t *testing.T) {
		ops, buf, _ := newTestOperators()
		buf.SetText("hello world")

		motion := MotionResult{Start: 0, End: 5, Inclusive: false}
		result, enterInsert := ops.ApplyOperator(OpChange, motion)

		assert.Equal(t, "hello", result)
		assert.True(t, enterInsert) // Should enter insert mode
		assert.Equal(t, " world", buf.Text())
	})

	t.Run("yank operator", func(t *testing.T) {
		ops, buf, _ := newTestOperators()
		buf.SetText("hello world")
		buf.SetCursor(3)

		motion := MotionResult{Start: 3, End: 8, Inclusive: false}
		result, enterInsert := ops.ApplyOperator(OpYank, motion)

		assert.Equal(t, "lo wo", result)
		assert.False(t, enterInsert)
		assert.Equal(t, "hello world", buf.Text()) // Unchanged
		assert.Equal(t, 3, buf.Cursor())           // Cursor restored
	})

	t.Run("no operator", func(t *testing.T) {
		ops, buf, _ := newTestOperators()
		buf.SetText("hello world")

		motion := MotionResult{Start: 0, End: 5, Inclusive: false}
		result, enterInsert := ops.ApplyOperator(OpNone, motion)

		assert.Equal(t, "", result)
		assert.False(t, enterInsert)
		assert.Equal(t, "hello world", buf.Text()) // Unchanged
	})
}

func TestOperators_DeleteChar(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		cursor         int
		count          int
		expectedText   string
		expectedDelete string
	}{
		{"delete one", "hello", 0, 1, "ello", "h"},
		{"delete multiple", "hello", 0, 3, "lo", "hel"},
		{"delete two from end", "hello", 3, 2, "hel", "lo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, buf, reg := newTestOperators()
			buf.SetText(tt.text)
			buf.SetCursor(tt.cursor)

			deleted := ops.DeleteChar(tt.count)

			assert.Equal(t, tt.expectedDelete, deleted)
			assert.Equal(t, tt.expectedText, buf.Text())
			assert.Equal(t, tt.expectedDelete, reg.Content)
		})
	}
}

func TestOperators_DeleteCharBefore(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		cursor         int
		count          int
		expectedText   string
		expectedDelete string
	}{
		{"delete one before", "hello", 2, 1, "hllo", "e"},
		{"delete multiple before", "hello", 4, 3, "ho", "ell"},
		{"delete at start", "hello", 0, 1, "hello", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, buf, reg := newTestOperators()
			buf.SetText(tt.text)
			buf.SetCursor(tt.cursor)

			deleted := ops.DeleteCharBefore(tt.count)

			assert.Equal(t, tt.expectedDelete, deleted)
			assert.Equal(t, tt.expectedText, buf.Text())
			assert.Equal(t, tt.expectedDelete, reg.Content)
		})
	}
}

func TestOperators_Substitute(t *testing.T) {
	ops, buf, _ := newTestOperators()
	buf.SetText("hello")
	buf.SetCursor(1)

	deleted := ops.Substitute(2)

	assert.Equal(t, "el", deleted)
	assert.Equal(t, "hlo", buf.Text())
}

func TestOperators_ReplaceChar(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		cursor         int
		char           rune
		count          int
		expectedText   string
		expectedCursor int
	}{
		{"replace one", "hello", 0, 'X', 1, "Xello", 0},
		{"replace multiple", "hello", 0, 'X', 3, "XXXlo", 2},
		{"replace at end", "hello", 4, 'X', 1, "hellX", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, buf, _ := newTestOperators()
			buf.SetText(tt.text)
			buf.SetCursor(tt.cursor)

			ops.ReplaceChar(tt.char, tt.count)

			assert.Equal(t, tt.expectedText, buf.Text())
			assert.Equal(t, tt.expectedCursor, buf.Cursor())
		})
	}
}
