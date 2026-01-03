package vi

// Operators handles d, c, y operators that work with motions
type Operators struct {
	buffer    *Buffer
	motions   *Motions
	finder    *CharFinder
	register  *Register
}

// NewOperators creates a new operators handler
func NewOperators(buf *Buffer, motions *Motions, finder *CharFinder, reg *Register) *Operators {
	return &Operators{
		buffer:   buf,
		motions:  motions,
		finder:   finder,
		register: reg,
	}
}

// Delete deletes the text covered by a motion (d{motion})
func (o *Operators) Delete(motion MotionResult) string {
	deleted := o.buffer.DeleteRange(motion.Start, motion.End)
	o.register.Content = deleted
	o.register.Linewise = false
	return deleted
}

// DeleteLine deletes the entire line (dd)
func (o *Operators) DeleteLine() string {
	start := 0
	end := o.buffer.Len()
	deleted := o.buffer.DeleteRange(start, end)
	o.register.Content = deleted
	o.register.Linewise = true
	return deleted
}

// Change deletes text and enters insert mode (c{motion})
// Returns the deleted text; caller should switch to insert mode
func (o *Operators) Change(motion MotionResult) string {
	deleted := o.buffer.DeleteRange(motion.Start, motion.End)
	o.register.Content = deleted
	o.register.Linewise = false
	return deleted
}

// ChangeLine changes the entire line (cc/S)
func (o *Operators) ChangeLine() string {
	return o.DeleteLine()
}

// ChangeToEnd changes from cursor to end of line (C)
func (o *Operators) ChangeToEnd() string {
	return o.buffer.DeleteToEnd()
}

// Yank copies text covered by a motion without deleting (y{motion})
func (o *Operators) Yank(motion MotionResult) string {
	start := min(motion.Start, motion.End)
	end := max(motion.Start, motion.End)
	yanked := o.buffer.SubstringBetween(start, end)
	o.register.Content = yanked
	o.register.Linewise = false
	return yanked
}

// YankLine yanks the entire line (yy/Y)
func (o *Operators) YankLine() string {
	yanked := o.buffer.Text()
	o.register.Content = yanked
	o.register.Linewise = true
	return yanked
}

// Paste pastes the register content after cursor (p)
func (o *Operators) Paste() {
	if o.register.Content == "" {
		return
	}

	// Move cursor right first for 'p' behavior
	if o.buffer.Len() > 0 {
		o.buffer.SetCursor(o.buffer.Cursor() + 1)
	}
	o.buffer.InsertString(o.register.Content)
	// Move back to last inserted character
	o.buffer.SetCursor(o.buffer.Cursor() - 1)
}

// PasteBefore pastes the register content before cursor (P)
func (o *Operators) PasteBefore() {
	if o.register.Content == "" {
		return
	}
	o.buffer.InsertString(o.register.Content)
	// Move back to first inserted character
	o.buffer.SetCursor(o.buffer.Cursor() - len([]rune(o.register.Content)))
}

// ApplyOperator applies an operator with a motion
func (o *Operators) ApplyOperator(op Operator, motion MotionResult) (string, bool) {
	switch op {
	case OpDelete:
		return o.Delete(motion), false
	case OpChange:
		return o.Change(motion), true // true = enter insert mode
	case OpYank:
		// Yank doesn't move cursor, restore original position
		o.buffer.SetCursor(min(motion.Start, motion.End))
		return o.Yank(motion), false
	default:
		return "", false
	}
}

// DeleteChar deletes character under cursor (x)
func (o *Operators) DeleteChar(count int) string {
	var deleted string
	for i := 0; i < count; i++ {
		r := o.buffer.DeleteChar()
		if r != 0 {
			deleted += string(r)
		}
	}
	o.register.Content = deleted
	o.register.Linewise = false
	return deleted
}

// DeleteCharBefore deletes character before cursor (X)
func (o *Operators) DeleteCharBefore(count int) string {
	var deleted string
	for i := 0; i < count; i++ {
		r := o.buffer.DeleteCharBefore()
		if r != 0 {
			deleted = string(r) + deleted
		}
	}
	o.register.Content = deleted
	o.register.Linewise = false
	return deleted
}

// Substitute deletes character and enters insert mode (s)
func (o *Operators) Substitute(count int) string {
	return o.DeleteChar(count)
}

// ReplaceChar replaces a single character (r<char>)
func (o *Operators) ReplaceChar(char rune, count int) {
	for i := 0; i < count; i++ {
		if o.buffer.Cursor() < o.buffer.Len() {
			o.buffer.Replace(char)
		}
	}
	// Move back one for replace behavior
	if o.buffer.Cursor() > 0 {
		o.buffer.SetCursor(o.buffer.Cursor() - 1)
	}
}
