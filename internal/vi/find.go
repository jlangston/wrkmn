package vi

// FindDirection represents the direction of a find operation
type FindDirection int

const (
	FindForward FindDirection = iota
	FindBackward
)

// FindType represents whether to find or till the character
type FindType int

const (
	FindTo FindType = iota // f/F - cursor lands ON character
	FindTill               // t/T - cursor lands BEFORE/AFTER character
)

// CharFinder handles f/F/t/T character finding
type CharFinder struct {
	buffer        *Buffer
	lastChar      rune
	lastDirection FindDirection
	lastType      FindType
}

// NewCharFinder creates a new character finder
func NewCharFinder(buf *Buffer) *CharFinder {
	return &CharFinder{buffer: buf}
}

// FindChar performs the f command - find character forward
func (f *CharFinder) FindChar(char rune, count int) MotionResult {
	f.lastChar = char
	f.lastDirection = FindForward
	f.lastType = FindTo
	return f.doFind(char, count, FindForward, FindTo)
}

// FindCharBack performs the F command - find character backward
func (f *CharFinder) FindCharBack(char rune, count int) MotionResult {
	f.lastChar = char
	f.lastDirection = FindBackward
	f.lastType = FindTo
	return f.doFind(char, count, FindBackward, FindTo)
}

// TillChar performs the t command - till character forward
func (f *CharFinder) TillChar(char rune, count int) MotionResult {
	f.lastChar = char
	f.lastDirection = FindForward
	f.lastType = FindTill
	return f.doFind(char, count, FindForward, FindTill)
}

// TillCharBack performs the T command - till character backward
func (f *CharFinder) TillCharBack(char rune, count int) MotionResult {
	f.lastChar = char
	f.lastDirection = FindBackward
	f.lastType = FindTill
	return f.doFind(char, count, FindBackward, FindTill)
}

// RepeatFind repeats the last find in the same direction (;)
func (f *CharFinder) RepeatFind(count int) MotionResult {
	if f.lastChar == 0 {
		start := f.buffer.Cursor()
		return MotionResult{Start: start, End: start, Inclusive: false}
	}
	return f.doFind(f.lastChar, count, f.lastDirection, f.lastType)
}

// RepeatFindReverse repeats the last find in the opposite direction (,)
func (f *CharFinder) RepeatFindReverse(count int) MotionResult {
	if f.lastChar == 0 {
		start := f.buffer.Cursor()
		return MotionResult{Start: start, End: start, Inclusive: false}
	}

	// Reverse direction
	dir := FindBackward
	if f.lastDirection == FindBackward {
		dir = FindForward
	}

	return f.doFind(f.lastChar, count, dir, f.lastType)
}

// doFind performs the actual character find
func (f *CharFinder) doFind(char rune, count int, dir FindDirection, findType FindType) MotionResult {
	start := f.buffer.Cursor()
	pos := start
	found := 0

	if dir == FindForward {
		// Search forward
		for i := start + 1; i < f.buffer.Len(); i++ {
			if f.buffer.RuneAt(i) == char {
				found++
				if found == count {
					pos = i
					if findType == FindTill {
						pos-- // Stop before the character
					}
					break
				}
			}
		}
	} else {
		// Search backward
		for i := start - 1; i >= 0; i-- {
			if f.buffer.RuneAt(i) == char {
				found++
				if found == count {
					pos = i
					if findType == FindTill {
						pos++ // Stop after the character
					}
					break
				}
			}
		}
	}

	if found < count {
		// Not found, don't move
		return MotionResult{Start: start, End: start, Inclusive: false}
	}

	f.buffer.SetCursor(pos)

	// For operators, the range should include the found character
	endPos := pos
	if dir == FindForward {
		endPos = pos + 1 // Include the found character
	}

	return MotionResult{
		Start:     start,
		End:       endPos,
		Inclusive: true,
	}
}

// HasLastFind returns true if there's a last find to repeat
func (f *CharFinder) HasLastFind() bool {
	return f.lastChar != 0
}
