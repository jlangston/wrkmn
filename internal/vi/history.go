package vi

import (
	"strings"
)

// History manages command history with navigation and search
type History struct {
	entries     []string // All history entries
	maxSize     int      // Maximum history size
	pos         int      // Current position in history (-1 = new entry)
	savedLine   string   // Line saved when navigating history
	searchMode  bool     // Currently searching
	searchTerm  string   // Current search term
	searchPos   int      // Position in search results
}

// NewHistory creates a new history ring
func NewHistory(maxSize int) *History {
	return &History{
		entries: make([]string, 0, maxSize),
		maxSize: maxSize,
		pos:     -1,
	}
}

// Add adds an entry to history
func (h *History) Add(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}

	// Remove duplicate if exists
	for i, e := range h.entries {
		if e == entry {
			h.entries = append(h.entries[:i], h.entries[i+1:]...)
			break
		}
	}

	// Add to end
	h.entries = append(h.entries, entry)

	// Trim if over capacity
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[1:]
	}

	// Reset position
	h.pos = -1
	h.savedLine = ""
}

// Prev returns the previous history entry (older)
// Returns the entry and true if successful, empty string and false otherwise
func (h *History) Prev(currentLine string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}

	// Save current line if we're starting to navigate
	if h.pos == -1 {
		h.savedLine = currentLine
		h.pos = len(h.entries) - 1
	} else if h.pos > 0 {
		h.pos--
	} else {
		// Already at oldest entry
		return h.entries[0], true
	}

	return h.entries[h.pos], true
}

// Next returns the next history entry (newer)
// Returns the entry and true if successful, saved line and false if at end
func (h *History) Next() (string, bool) {
	if h.pos == -1 {
		return h.savedLine, false
	}

	h.pos++

	if h.pos >= len(h.entries) {
		// Back to the saved line
		h.pos = -1
		return h.savedLine, false
	}

	return h.entries[h.pos], true
}

// Reset resets the history position
func (h *History) Reset() {
	h.pos = -1
	h.savedLine = ""
	h.searchMode = false
	h.searchTerm = ""
}

// Search searches history for entries containing pattern
// direction: -1 for backward (older), 1 for forward (newer)
func (h *History) Search(pattern string, direction int) (string, bool) {
	if pattern == "" || len(h.entries) == 0 {
		return "", false
	}

	pattern = strings.ToLower(pattern)

	if direction < 0 {
		// Search backward (older entries)
		start := len(h.entries) - 1
		if h.searchMode && h.searchTerm == pattern && h.searchPos > 0 {
			start = h.searchPos - 1
		}

		for i := start; i >= 0; i-- {
			if strings.Contains(strings.ToLower(h.entries[i]), pattern) {
				h.searchMode = true
				h.searchTerm = pattern
				h.searchPos = i
				h.pos = i
				return h.entries[i], true
			}
		}
	} else {
		// Search forward (newer entries)
		start := 0
		if h.searchMode && h.searchTerm == pattern && h.searchPos < len(h.entries)-1 {
			start = h.searchPos + 1
		}

		for i := start; i < len(h.entries); i++ {
			if strings.Contains(strings.ToLower(h.entries[i]), pattern) {
				h.searchMode = true
				h.searchTerm = pattern
				h.searchPos = i
				h.pos = i
				return h.entries[i], true
			}
		}
	}

	return "", false
}

// SearchNext continues the search in the same direction (n)
func (h *History) SearchNext() (string, bool) {
	if !h.searchMode || h.searchTerm == "" {
		return "", false
	}
	return h.Search(h.searchTerm, -1) // Default to backward
}

// SearchPrev continues the search in the opposite direction (N)
func (h *History) SearchPrev() (string, bool) {
	if !h.searchMode || h.searchTerm == "" {
		return "", false
	}
	return h.Search(h.searchTerm, 1)
}

// GetSearchTerm returns the current search term
func (h *History) GetSearchTerm() string {
	return h.searchTerm
}

// IsSearchMode returns true if currently in search mode
func (h *History) IsSearchMode() bool {
	return h.searchMode
}

// Len returns the number of history entries
func (h *History) Len() int {
	return len(h.entries)
}

// GetAll returns all history entries
func (h *History) GetAll() []string {
	result := make([]string, len(h.entries))
	copy(result, h.entries)
	return result
}

// Clear clears all history
func (h *History) Clear() {
	h.entries = h.entries[:0]
	h.Reset()
}
