package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
)

// HelpBinding represents a single keybinding with description
type HelpBinding struct {
	Key  string
	Desc string
}

// HelpSection represents a group of related keybindings
type HelpSection struct {
	Title    string
	Bindings []HelpBinding
}

// HelpOverlay renders a help overlay with keybindings
type HelpOverlay struct {
	sections []HelpSection
	visible  bool
	width    int
	height   int
	scroll   int
}

// NewHelpOverlay creates a new help overlay
func NewHelpOverlay() *HelpOverlay {
	return &HelpOverlay{
		sections: defaultHelpSections(),
	}
}

// defaultHelpSections returns the default help sections
func defaultHelpSections() []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Bindings: []HelpBinding{
				{"j/↓", "Move down"},
				{"k/↑", "Move up"},
				{"g", "Go to top"},
				{"G", "Go to bottom"},
				{"/", "Search"},
				{"?", "Toggle help"},
			},
		},
		{
			Title: "Process Control",
			Bindings: []HelpBinding{
				{"d", "Start (dev)"},
				{"x", "Stop process"},
				{"a", "Start all"},
				{"z", "Stop all"},
				{"r", "Restart process"},
			},
		},
		{
			Title: "Views",
			Bindings: []HelpBinding{
				{"l/Enter", "View logs"},
				{"D", "Debug (CF Workers)"},
				{"m", "Monitor view"},
				{"Esc/q", "Back/Quit"},
			},
		},
		{
			Title: "VI Mode (REPL)",
			Bindings: []HelpBinding{
				{"i/a/A", "Insert mode"},
				{"Esc", "Normal mode"},
				{"h/l", "Move left/right"},
				{"w/b", "Word forward/back"},
				{"0/$", "Line start/end"},
				{"d/c/y", "Delete/change/yank"},
				{"p/P", "Paste after/before"},
				{"u", "Undo"},
			},
		},
		{
			Title: "Debug (CF Workers)",
			Bindings: []HelpBinding{
				{"F5", "Continue/Resume"},
				{"F9", "Toggle breakpoint"},
				{"F10", "Step over"},
				{"F11", "Step into"},
				{"S-F11", "Step out"},
				{"Tab", "Switch panel"},
			},
		},
	}
}

// SetSections allows customizing the help sections
func (h *HelpOverlay) SetSections(sections []HelpSection) {
	h.sections = sections
}

// Toggle toggles visibility of the help overlay
func (h *HelpOverlay) Toggle() {
	h.visible = !h.visible
	h.scroll = 0
}

// Show shows the help overlay
func (h *HelpOverlay) Show() {
	h.visible = true
	h.scroll = 0
}

// Hide hides the help overlay
func (h *HelpOverlay) Hide() {
	h.visible = false
}

// Visible returns whether the help overlay is visible
func (h *HelpOverlay) Visible() bool {
	return h.visible
}

// SetSize updates the overlay dimensions
func (h *HelpOverlay) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// Update handles key events for the help overlay
func (h *HelpOverlay) Update(msg tea.Msg) tea.Cmd {
	if !h.visible {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "?", "q", "esc":
			h.Hide()
		case "j", "down":
			h.scroll++
		case "k", "up":
			if h.scroll > 0 {
				h.scroll--
			}
		}
	}

	return nil
}

// View renders the help overlay
func (h *HelpOverlay) View() string {
	if !h.visible {
		return ""
	}

	t := theme.Current
	styles := NewStyles()

	// Calculate overlay dimensions
	overlayWidth := min(80, h.width-4)
	overlayHeight := min(30, h.height-4)

	// Build content
	var sections []string

	titleStyle := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		MarginBottom(1)

	for _, section := range h.sections {
		var lines []string
		lines = append(lines, titleStyle.Render(section.Title))

		for _, binding := range section.Bindings {
			key := styles.HelpKey.Width(12).Render(binding.Key)
			desc := styles.HelpDesc.Render(binding.Desc)
			lines = append(lines, key+desc)
		}

		sections = append(sections, strings.Join(lines, "\n"))
	}

	content := strings.Join(sections, "\n\n")

	// Apply scroll
	lines := strings.Split(content, "\n")
	if h.scroll > 0 && h.scroll < len(lines) {
		lines = lines[h.scroll:]
	}
	if len(lines) > overlayHeight-4 {
		lines = lines[:overlayHeight-4]
	}
	content = strings.Join(lines, "\n")

	// Overlay style
	overlayStyle := lipgloss.NewStyle().
		Width(overlayWidth).
		Height(overlayHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).
		Padding(1, 2).
		Background(t.Background)

	header := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Render("Help • Press ? or Esc to close")

	overlay := lipgloss.JoinVertical(lipgloss.Left, header, "", content)

	return overlayStyle.Render(overlay)
}

// RenderInline renders a compact inline help bar
func (h *HelpOverlay) RenderInline(bindings []HelpBinding) string {
	styles := NewStyles()
	var parts []string

	for _, b := range bindings {
		key := styles.HelpKey.Render(b.Key)
		desc := styles.HelpDesc.Render(b.Desc)
		parts = append(parts, key+":"+desc)
	}

	sep := styles.HelpSep.Render(" • ")
	return strings.Join(parts, sep)
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
