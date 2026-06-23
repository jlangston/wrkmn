package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
)

// confirmDialog is a minimal y/n prompt. The owning model stores an onYes
// closure and runs it when the user confirms.
type confirmDialog struct {
	visible bool
	message string
	onYes   func(m TableModel) (TableModel, tea.Cmd)
}

func (c *confirmDialog) Show(message string, onYes func(m TableModel) (TableModel, tea.Cmd)) {
	c.visible = true
	c.message = message
	c.onYes = onYes
}

func (c *confirmDialog) Hide() {
	c.visible = false
	c.onYes = nil
}

func (c *confirmDialog) Visible() bool { return c.visible }

func (c *confirmDialog) View(width int) string {
	if !c.visible {
		return ""
	}
	t := theme.Current
	overlayWidth := min(60, width-4)
	header := lipgloss.NewStyle().Foreground(t.Warning).Bold(true).Render("Confirm full-remote start")
	body := lipgloss.NewStyle().Foreground(t.Foreground).Width(overlayWidth - 4).Render(c.message)
	footer := lipgloss.NewStyle().Foreground(t.Secondary).Render("y: start • n/Esc: cancel")
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(overlayWidth).Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Warning).Padding(1, 2).Background(t.Background).Render(content)
}
