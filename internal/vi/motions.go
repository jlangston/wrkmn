package vi

import (
	"unicode"
)

// MotionResult contains the result of executing a motion
type MotionResult struct {
	Start    int  // Start position (original cursor)
	End      int  // End position (new cursor)
	Inclusive bool // Whether end position is inclusive (for operators)
}

// Motions provides motion implementations
type Motions struct {
	buffer *Buffer
}

// NewMotions creates a new Motions instance
func NewMotions(buf *Buffer) *Motions {
	return &Motions{buffer: buf}
}

// Left moves cursor left (h)
func (m *Motions) Left(count int) MotionResult {
	start := m.buffer.Cursor()
	end := max(0, start-count)
	m.buffer.SetCursor(end)
	return MotionResult{Start: start, End: end, Inclusive: false}
}

// Right moves cursor right (l)
func (m *Motions) Right(count int) MotionResult {
	start := m.buffer.Cursor()
	maxPos := max(0, m.buffer.Len()-1)
	end := min(maxPos, start+count)
	m.buffer.SetCursor(end)
	return MotionResult{Start: start, End: end, Inclusive: false}
}

// LineStart moves to start of line (0)
func (m *Motions) LineStart() MotionResult {
	start := m.buffer.Cursor()
	m.buffer.SetCursor(0)
	return MotionResult{Start: start, End: 0, Inclusive: false}
}

// LineEnd moves to end of line ($)
func (m *Motions) LineEnd() MotionResult {
	start := m.buffer.Cursor()
	end := max(0, m.buffer.Len()-1)
	m.buffer.SetCursor(end)
	return MotionResult{Start: start, End: m.buffer.Len(), Inclusive: true}
}

// FirstNonBlank moves to first non-blank character (^)
func (m *Motions) FirstNonBlank() MotionResult {
	start := m.buffer.Cursor()
	end := 0
	for i := 0; i < m.buffer.Len(); i++ {
		if !unicode.IsSpace(m.buffer.RuneAt(i)) {
			end = i
			break
		}
	}
	m.buffer.SetCursor(end)
	return MotionResult{Start: start, End: end, Inclusive: false}
}

// WordForward moves to start of next word (w)
func (m *Motions) WordForward(count int) MotionResult {
	start := m.buffer.Cursor()
	pos := start

	for i := 0; i < count; i++ {
		pos = m.nextWordStart(pos, false)
	}

	m.buffer.SetCursor(pos)
	return MotionResult{Start: start, End: pos, Inclusive: false}
}

// WORDForward moves to start of next WORD (W)
func (m *Motions) WORDForward(count int) MotionResult {
	start := m.buffer.Cursor()
	pos := start

	for i := 0; i < count; i++ {
		pos = m.nextWordStart(pos, true)
	}

	m.buffer.SetCursor(pos)
	return MotionResult{Start: start, End: pos, Inclusive: false}
}

// WordBackward moves to start of previous word (b)
func (m *Motions) WordBackward(count int) MotionResult {
	start := m.buffer.Cursor()
	pos := start

	for i := 0; i < count; i++ {
		pos = m.prevWordStart(pos, false)
	}

	m.buffer.SetCursor(pos)
	return MotionResult{Start: start, End: pos, Inclusive: false}
}

// WORDBackward moves to start of previous WORD (B)
func (m *Motions) WORDBackward(count int) MotionResult {
	start := m.buffer.Cursor()
	pos := start

	for i := 0; i < count; i++ {
		pos = m.prevWordStart(pos, true)
	}

	m.buffer.SetCursor(pos)
	return MotionResult{Start: start, End: pos, Inclusive: false}
}

// WordEnd moves to end of current/next word (e)
func (m *Motions) WordEnd(count int) MotionResult {
	start := m.buffer.Cursor()
	pos := start

	for i := 0; i < count; i++ {
		pos = m.nextWordEnd(pos, false)
	}

	m.buffer.SetCursor(pos)
	return MotionResult{Start: start, End: pos + 1, Inclusive: true}
}

// WORDEnd moves to end of current/next WORD (E)
func (m *Motions) WORDEnd(count int) MotionResult {
	start := m.buffer.Cursor()
	pos := start

	for i := 0; i < count; i++ {
		pos = m.nextWordEnd(pos, true)
	}

	m.buffer.SetCursor(pos)
	return MotionResult{Start: start, End: pos + 1, Inclusive: true}
}

// MatchBracket moves to matching bracket (%)
func (m *Motions) MatchBracket() MotionResult {
	start := m.buffer.Cursor()

	// Find bracket under cursor or ahead
	pairs := map[rune]rune{
		'(': ')', ')': '(',
		'[': ']', ']': '[',
		'{': '}', '}': '{',
		'<': '>', '>': '<',
	}

	openBrackets := map[rune]bool{'(': true, '[': true, '{': true, '<': true}

	// Find a bracket starting from cursor
	bracketPos := -1
	var bracket rune
	for i := start; i < m.buffer.Len(); i++ {
		r := m.buffer.RuneAt(i)
		if _, ok := pairs[r]; ok {
			bracketPos = i
			bracket = r
			break
		}
	}

	if bracketPos == -1 {
		return MotionResult{Start: start, End: start, Inclusive: false}
	}

	match := pairs[bracket]
	isOpen := openBrackets[bracket]
	depth := 1

	if isOpen {
		// Search forward
		for i := bracketPos + 1; i < m.buffer.Len(); i++ {
			r := m.buffer.RuneAt(i)
			if r == bracket {
				depth++
			} else if r == match {
				depth--
				if depth == 0 {
					m.buffer.SetCursor(i)
					return MotionResult{Start: start, End: i, Inclusive: true}
				}
			}
		}
	} else {
		// Search backward
		for i := bracketPos - 1; i >= 0; i-- {
			r := m.buffer.RuneAt(i)
			if r == bracket {
				depth++
			} else if r == match {
				depth--
				if depth == 0 {
					m.buffer.SetCursor(i)
					return MotionResult{Start: start, End: i, Inclusive: true}
				}
			}
		}
	}

	return MotionResult{Start: start, End: start, Inclusive: false}
}

// InnerWord returns the range of the inner word (iw)
func (m *Motions) InnerWord() MotionResult {
	start := m.buffer.Cursor()

	// Find word boundaries
	wordStart := start
	wordEnd := start

	// If on whitespace, select the whitespace
	if start < m.buffer.Len() && unicode.IsSpace(m.buffer.RuneAt(start)) {
		for wordStart > 0 && unicode.IsSpace(m.buffer.RuneAt(wordStart-1)) {
			wordStart--
		}
		for wordEnd < m.buffer.Len() && unicode.IsSpace(m.buffer.RuneAt(wordEnd)) {
			wordEnd++
		}
	} else {
		// Find word boundaries
		for wordStart > 0 && IsWordChar(m.buffer.RuneAt(wordStart-1)) {
			wordStart--
		}
		for wordEnd < m.buffer.Len() && IsWordChar(m.buffer.RuneAt(wordEnd)) {
			wordEnd++
		}
	}

	return MotionResult{Start: wordStart, End: wordEnd, Inclusive: true}
}

// AWord returns the range of a word including surrounding space (aw)
func (m *Motions) AWord() MotionResult {
	inner := m.InnerWord()

	// Extend to include trailing or leading whitespace
	end := inner.End
	for end < m.buffer.Len() && unicode.IsSpace(m.buffer.RuneAt(end)) {
		end++
	}

	if end == inner.End && inner.Start > 0 {
		// No trailing space, try leading
		start := inner.Start
		for start > 0 && unicode.IsSpace(m.buffer.RuneAt(start-1)) {
			start--
		}
		return MotionResult{Start: start, End: inner.End, Inclusive: true}
	}

	return MotionResult{Start: inner.Start, End: end, Inclusive: true}
}

// Helper methods

// nextWordStart finds the start of the next word
func (m *Motions) nextWordStart(pos int, bigWord bool) int {
	if pos >= m.buffer.Len()-1 {
		return m.buffer.Len() - 1
	}

	isWordChar := IsWordChar
	if bigWord {
		isWordChar = IsWORDChar
	}

	// Move past current word
	for pos < m.buffer.Len() && isWordChar(m.buffer.RuneAt(pos)) {
		pos++
	}

	// Move past whitespace/punctuation
	for pos < m.buffer.Len() && !isWordChar(m.buffer.RuneAt(pos)) {
		pos++
	}

	return min(pos, m.buffer.Len()-1)
}

// prevWordStart finds the start of the previous word
func (m *Motions) prevWordStart(pos int, bigWord bool) int {
	if pos == 0 {
		return 0
	}

	isWordChar := IsWordChar
	if bigWord {
		isWordChar = IsWORDChar
	}

	pos--

	// Move past whitespace/punctuation
	for pos > 0 && !isWordChar(m.buffer.RuneAt(pos)) {
		pos--
	}

	// Move to start of word
	for pos > 0 && isWordChar(m.buffer.RuneAt(pos-1)) {
		pos--
	}

	return pos
}

// nextWordEnd finds the end of the current/next word
func (m *Motions) nextWordEnd(pos int, bigWord bool) int {
	if pos >= m.buffer.Len()-1 {
		return m.buffer.Len() - 1
	}

	isWordChar := IsWordChar
	if bigWord {
		isWordChar = IsWORDChar
	}

	pos++

	// Skip whitespace
	for pos < m.buffer.Len() && unicode.IsSpace(m.buffer.RuneAt(pos)) {
		pos++
	}

	// Move to end of word
	for pos < m.buffer.Len()-1 && isWordChar(m.buffer.RuneAt(pos+1)) {
		pos++
	}

	return min(pos, m.buffer.Len()-1)
}
