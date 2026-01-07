package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"wrkmn/internal/debug"
)

func TestNewAutocompletePopup(t *testing.T) {
	popup := NewAutocompletePopup()
	assert.NotNil(t, popup)
	assert.False(t, popup.Visible())
	assert.Equal(t, 10, popup.maxHeight)
}

func TestAutocompletePopup_Show(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "foo", Label: "foo", Type: "variable"},
		{Text: "bar", Label: "bar", Type: "function"},
	}

	popup.Show(completions, 10, 5)

	assert.True(t, popup.Visible())
	assert.Equal(t, 2, len(popup.completions))
	assert.Equal(t, 0, popup.cursor)
	assert.Equal(t, 0, popup.scroll)
	assert.Equal(t, 10, popup.x)
	assert.Equal(t, 5, popup.y)
}

func TestAutocompletePopup_ShowEmpty(t *testing.T) {
	popup := NewAutocompletePopup()

	// Showing empty completions should hide the popup
	popup.Show([]debug.Completion{}, 10, 5)
	assert.False(t, popup.Visible())

	// Showing nil should also hide
	popup.Show(nil, 10, 5)
	assert.False(t, popup.Visible())
}

func TestAutocompletePopup_Hide(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "foo", Label: "foo"},
	}

	popup.Show(completions, 10, 5)
	assert.True(t, popup.Visible())

	popup.Hide()
	assert.False(t, popup.Visible())
	assert.Nil(t, popup.completions)
}

func TestAutocompletePopup_Next(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "a"},
		{Text: "b"},
		{Text: "c"},
	}

	popup.Show(completions, 0, 0)

	assert.Equal(t, 0, popup.cursor)

	popup.Next()
	assert.Equal(t, 1, popup.cursor)

	popup.Next()
	assert.Equal(t, 2, popup.cursor)

	// Should wrap around
	popup.Next()
	assert.Equal(t, 0, popup.cursor)
}

func TestAutocompletePopup_Prev(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "a"},
		{Text: "b"},
		{Text: "c"},
	}

	popup.Show(completions, 0, 0)

	assert.Equal(t, 0, popup.cursor)

	// Should wrap to end
	popup.Prev()
	assert.Equal(t, 2, popup.cursor)

	popup.Prev()
	assert.Equal(t, 1, popup.cursor)

	popup.Prev()
	assert.Equal(t, 0, popup.cursor)
}

func TestAutocompletePopup_Selected(t *testing.T) {
	popup := NewAutocompletePopup()

	// No selection when not visible
	assert.Nil(t, popup.Selected())

	completions := []debug.Completion{
		{Text: "foo"},
		{Text: "bar"},
	}

	popup.Show(completions, 0, 0)

	selected := popup.Selected()
	assert.NotNil(t, selected)
	assert.Equal(t, "foo", selected.Text)

	popup.Next()
	selected = popup.Selected()
	assert.Equal(t, "bar", selected.Text)
}

func TestAutocompletePopup_Scrolling(t *testing.T) {
	popup := NewAutocompletePopup()
	popup.SetMaxHeight(3)

	// Create more completions than maxHeight
	completions := make([]debug.Completion, 10)
	for i := 0; i < 10; i++ {
		completions[i] = debug.Completion{Text: string(rune('a' + i))}
	}

	popup.Show(completions, 0, 0)
	assert.Equal(t, 0, popup.scroll)

	// Navigate past visible area
	popup.Next() // cursor = 1
	popup.Next() // cursor = 2
	popup.Next() // cursor = 3, should scroll
	assert.Equal(t, 1, popup.scroll)
}

func TestAutocompletePopup_Update_Tab(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "a"},
		{Text: "b"},
	}

	popup.Show(completions, 0, 0)

	// Tab should move to next
	handled, selected := popup.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.True(t, handled)
	assert.Nil(t, selected)
	assert.Equal(t, 1, popup.cursor)
}

func TestAutocompletePopup_Update_ShiftTab(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "a"},
		{Text: "b"},
	}

	popup.Show(completions, 0, 0)
	popup.cursor = 1 // Start at second item

	// Shift+Tab should move to prev
	handled, selected := popup.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.True(t, handled)
	assert.Nil(t, selected)
	assert.Equal(t, 0, popup.cursor)
}

func TestAutocompletePopup_Update_Enter(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "foo"},
		{Text: "bar"},
	}

	popup.Show(completions, 0, 0)

	// Enter should return selected and hide
	handled, selected := popup.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, handled)
	assert.NotNil(t, selected)
	assert.Equal(t, "foo", selected.Text)
	assert.False(t, popup.Visible())
}

func TestAutocompletePopup_Update_Escape(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "foo"},
	}

	popup.Show(completions, 0, 0)

	// Escape should hide without selecting
	handled, selected := popup.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.True(t, handled)
	assert.Nil(t, selected)
	assert.False(t, popup.Visible())
}

func TestAutocompletePopup_Update_NotVisible(t *testing.T) {
	popup := NewAutocompletePopup()

	// When not visible, should not handle any keys
	handled, selected := popup.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.False(t, handled)
	assert.Nil(t, selected)
}

func TestAutocompletePopup_Update_Arrows(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "a"},
		{Text: "b"},
	}

	popup.Show(completions, 0, 0)

	// Down arrow
	handled, _ := popup.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.True(t, handled)
	assert.Equal(t, 1, popup.cursor)

	// Up arrow
	handled, _ = popup.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.True(t, handled)
	assert.Equal(t, 0, popup.cursor)
}

func TestAutocompletePopup_GetPosition(t *testing.T) {
	popup := NewAutocompletePopup()

	completions := []debug.Completion{
		{Text: "foo"},
	}

	popup.Show(completions, 15, 20)

	x, y := popup.GetPosition()
	assert.Equal(t, 15, x)
	assert.Equal(t, 20, y)
}

func TestAutocompletePopup_SetMaxHeight(t *testing.T) {
	popup := NewAutocompletePopup()

	popup.SetMaxHeight(5)
	assert.Equal(t, 5, popup.maxHeight)

	// Should ignore invalid values
	popup.SetMaxHeight(0)
	assert.Equal(t, 5, popup.maxHeight)

	popup.SetMaxHeight(-1)
	assert.Equal(t, 5, popup.maxHeight)
}

func TestAutocompletePopup_View(t *testing.T) {
	popup := NewAutocompletePopup()

	// Empty when not visible
	assert.Equal(t, "", popup.View())

	completions := []debug.Completion{
		{Text: "foo", Label: "foo", Type: "variable", Description: "string"},
		{Text: "bar", Label: "bar()", Type: "function", Description: "function"},
	}

	popup.Show(completions, 0, 0)

	view := popup.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "foo")
	assert.Contains(t, view, "bar")
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 10, "hello     "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"}, // Doesn't truncate
		{"", 5, "     "},
	}

	for _, tt := range tests {
		result := padRight(tt.input, tt.width)
		assert.Equal(t, tt.expected, result)
	}
}
