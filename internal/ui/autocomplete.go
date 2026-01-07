package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/debug"
	"wrkmn/internal/theme"
)

// AutocompletePopup displays completion suggestions
type AutocompletePopup struct {
	visible     bool
	completions []debug.Completion
	cursor      int
	x, y        int // Position relative to input
	maxHeight   int
	width       int
	scroll      int
}

// NewAutocompletePopup creates a new autocomplete popup
func NewAutocompletePopup() *AutocompletePopup {
	return &AutocompletePopup{
		maxHeight: 10,
	}
}

// Show displays the popup with completions
func (a *AutocompletePopup) Show(completions []debug.Completion, x, y int) {
	if len(completions) == 0 {
		a.visible = false
		return
	}

	a.visible = true
	a.completions = completions
	a.cursor = 0
	a.scroll = 0
	a.x = x
	a.y = y

	// Calculate width based on longest completion
	maxWidth := 20
	for _, c := range completions {
		labelLen := len(c.Label) + len(c.Description) + 3 // space + type
		if labelLen > maxWidth {
			maxWidth = labelLen
		}
	}
	a.width = min(maxWidth+4, 60) // Cap at 60 chars
}

// Hide closes the popup
func (a *AutocompletePopup) Hide() {
	a.visible = false
	a.completions = nil
}

// Visible returns whether the popup is visible
func (a *AutocompletePopup) Visible() bool {
	return a.visible
}

// Next moves to the next completion
func (a *AutocompletePopup) Next() {
	if !a.visible || len(a.completions) == 0 {
		return
	}

	a.cursor++
	if a.cursor >= len(a.completions) {
		a.cursor = 0
		a.scroll = 0
	}

	// Scroll if needed
	if a.cursor >= a.scroll+a.maxHeight {
		a.scroll = a.cursor - a.maxHeight + 1
	}
}

// Prev moves to the previous completion
func (a *AutocompletePopup) Prev() {
	if !a.visible || len(a.completions) == 0 {
		return
	}

	a.cursor--
	if a.cursor < 0 {
		a.cursor = len(a.completions) - 1
		if len(a.completions) > a.maxHeight {
			a.scroll = len(a.completions) - a.maxHeight
		}
	}

	// Scroll if needed
	if a.cursor < a.scroll {
		a.scroll = a.cursor
	}
}

// Selected returns the currently selected completion
func (a *AutocompletePopup) Selected() *debug.Completion {
	if !a.visible || len(a.completions) == 0 {
		return nil
	}
	return &a.completions[a.cursor]
}

// Update handles key events
func (a *AutocompletePopup) Update(msg tea.Msg) (handled bool, selected *debug.Completion) {
	if !a.visible {
		return false, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab:
			a.Next()
			return true, nil

		case tea.KeyShiftTab:
			a.Prev()
			return true, nil

		case tea.KeyDown:
			a.Next()
			return true, nil

		case tea.KeyUp:
			a.Prev()
			return true, nil

		case tea.KeyEnter:
			selected := a.Selected()
			a.Hide()
			return true, selected

		case tea.KeyEscape:
			a.Hide()
			return true, nil
		}

		// Check for specific keys
		switch msg.String() {
		case "ctrl+n":
			a.Next()
			return true, nil
		case "ctrl+p":
			a.Prev()
			return true, nil
		}
	}

	return false, nil
}

// View renders the popup
func (a *AutocompletePopup) View() string {
	if !a.visible || len(a.completions) == 0 {
		return ""
	}

	t := theme.Current

	// Styles
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).
		Background(t.Background).
		Padding(0, 1)

	selectedStyle := lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(t.Background).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(t.Foreground)

	typeStyle := lipgloss.NewStyle().
		Foreground(t.Secondary)

	funcStyle := lipgloss.NewStyle().
		Foreground(t.Warning)

	varStyle := lipgloss.NewStyle().
		Foreground(t.Secondary)

	// Calculate visible range
	visibleCount := min(a.maxHeight, len(a.completions))
	endIdx := a.scroll + visibleCount
	if endIdx > len(a.completions) {
		endIdx = len(a.completions)
	}

	var lines []string

	for i := a.scroll; i < endIdx; i++ {
		comp := a.completions[i]

		// Build label with icon based on type
		var icon string
		var labelStyled string

		switch comp.Type {
		case "function", "method":
			icon = "ƒ "
			labelStyled = funcStyle.Render(comp.Label)
		case "variable":
			icon = "ν "
			labelStyled = varStyle.Render(comp.Label)
		case "property":
			icon = "• "
			labelStyled = normalStyle.Render(comp.Label)
		default:
			icon = "  "
			labelStyled = normalStyle.Render(comp.Label)
		}

		// Build the line
		desc := ""
		if comp.Description != "" {
			desc = " " + typeStyle.Render(comp.Description)
		}

		line := icon + labelStyled + desc

		// Truncate if too long
		if lipgloss.Width(line) > a.width-4 {
			line = line[:a.width-7] + "..."
		}

		// Pad to consistent width
		line = padRight(line, a.width-4)

		// Apply selection style
		if i == a.cursor {
			line = selectedStyle.Render(line)
		}

		lines = append(lines, line)
	}

	// Add scroll indicators
	content := strings.Join(lines, "\n")

	if a.scroll > 0 {
		content = typeStyle.Render("↑ more") + "\n" + content
	}
	if endIdx < len(a.completions) {
		content = content + "\n" + typeStyle.Render("↓ more")
	}

	return containerStyle.Render(content)
}

// GetPosition returns the x, y position for the popup
func (a *AutocompletePopup) GetPosition() (int, int) {
	return a.x, a.y
}

// SetMaxHeight sets the maximum height of the popup
func (a *AutocompletePopup) SetMaxHeight(h int) {
	if h > 0 {
		a.maxHeight = h
	}
}

// padRight pads a string to the given width
func padRight(s string, width int) string {
	currentWidth := lipgloss.Width(s)
	if currentWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-currentWidth)
}
