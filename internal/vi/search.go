package vi

import (
	"regexp"
	"strings"
)

// Search handles /pattern and ?pattern searching
type Search struct {
	buffer       *Buffer
	history      *History
	pattern      string
	lastPattern  string
	direction    int // 1 = forward, -1 = backward
	matchPos     int // Current match position
	isActive     bool
	inputBuffer  string
}

// NewSearch creates a new search handler
func NewSearch(buf *Buffer, hist *History) *Search {
	return &Search{
		buffer:    buf,
		history:   hist,
		direction: 1,
		matchPos:  -1,
	}
}

// StartForward starts a forward search (/)
func (s *Search) StartForward() {
	s.isActive = true
	s.direction = 1
	s.inputBuffer = ""
}

// StartBackward starts a backward search (?)
func (s *Search) StartBackward() {
	s.isActive = true
	s.direction = -1
	s.inputBuffer = ""
}

// IsActive returns true if search input is active
func (s *Search) IsActive() bool {
	return s.isActive
}

// GetInputBuffer returns the current search input
func (s *Search) GetInputBuffer() string {
	return s.inputBuffer
}

// AddChar adds a character to the search pattern
func (s *Search) AddChar(r rune) {
	s.inputBuffer += string(r)
}

// RemoveChar removes the last character from the search pattern
func (s *Search) RemoveChar() bool {
	if len(s.inputBuffer) == 0 {
		return false
	}
	runes := []rune(s.inputBuffer)
	s.inputBuffer = string(runes[:len(runes)-1])
	return true
}

// Cancel cancels the search
func (s *Search) Cancel() {
	s.isActive = false
	s.inputBuffer = ""
}

// Execute performs the search with the current pattern
func (s *Search) Execute() bool {
	s.isActive = false
	s.pattern = s.inputBuffer
	s.inputBuffer = ""

	if s.pattern == "" {
		// Use last pattern if empty
		s.pattern = s.lastPattern
	}

	if s.pattern == "" {
		return false
	}

	s.lastPattern = s.pattern

	return s.doSearch()
}

// Next finds the next match (n)
func (s *Search) Next() bool {
	if s.lastPattern == "" {
		return false
	}
	s.pattern = s.lastPattern
	return s.doSearch()
}

// Prev finds the previous match (N) - searches in opposite direction
func (s *Search) Prev() bool {
	if s.lastPattern == "" {
		return false
	}
	s.pattern = s.lastPattern
	s.direction = -s.direction
	result := s.doSearch()
	s.direction = -s.direction // Restore original direction
	return result
}

// doSearch performs the actual search
func (s *Search) doSearch() bool {
	text := s.buffer.Text()
	if text == "" || s.pattern == "" {
		return false
	}

	// Try as regex first, fall back to literal
	var positions []int
	re, err := regexp.Compile("(?i)" + s.pattern) // Case insensitive
	if err == nil {
		// Regex search
		matches := re.FindAllStringIndex(text, -1)
		for _, m := range matches {
			positions = append(positions, m[0])
		}
	} else {
		// Literal search (case insensitive)
		lowerText := strings.ToLower(text)
		lowerPattern := strings.ToLower(s.pattern)
		pos := 0
		for {
			idx := strings.Index(lowerText[pos:], lowerPattern)
			if idx == -1 {
				break
			}
			positions = append(positions, pos+idx)
			pos += idx + 1
		}
	}

	if len(positions) == 0 {
		return false
	}

	cursor := s.buffer.Cursor()

	if s.direction > 0 {
		// Forward search - find first match after cursor
		for _, pos := range positions {
			if pos > cursor {
				s.buffer.SetCursor(pos)
				s.matchPos = pos
				return true
			}
		}
		// Wrap around
		s.buffer.SetCursor(positions[0])
		s.matchPos = positions[0]
		return true
	} else {
		// Backward search - find first match before cursor
		for i := len(positions) - 1; i >= 0; i-- {
			if positions[i] < cursor {
				s.buffer.SetCursor(positions[i])
				s.matchPos = positions[i]
				return true
			}
		}
		// Wrap around
		s.buffer.SetCursor(positions[len(positions)-1])
		s.matchPos = positions[len(positions)-1]
		return true
	}
}

// GetLastPattern returns the last search pattern
func (s *Search) GetLastPattern() string {
	return s.lastPattern
}

// GetDirection returns the search direction as a string
func (s *Search) GetDirection() string {
	if s.direction > 0 {
		return "/"
	}
	return "?"
}
