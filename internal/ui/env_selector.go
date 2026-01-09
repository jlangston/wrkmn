package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
	"wrkmn/internal/worker"
)

// EnvSelector displays available environments for a worker
type EnvSelector struct {
	visible bool
	worker  *worker.Worker
	cursor  int
	width   int
	height  int

	// Callback when environment is selected
	onSelect func(w *worker.Worker, env string)
}

// NewEnvSelector creates a new environment selector popup
func NewEnvSelector() *EnvSelector {
	return &EnvSelector{}
}

// Show displays the popup for the given worker
func (e *EnvSelector) Show(w *worker.Worker) {
	if w == nil || len(w.Environments) == 0 {
		return
	}

	e.visible = true
	e.worker = w
	e.cursor = 0

	// Find current selection
	for i, env := range w.Environments {
		if env == w.SelectedEnv {
			e.cursor = i
			break
		}
	}
}

// Hide closes the popup
func (e *EnvSelector) Hide() {
	e.visible = false
}

// Visible returns whether the popup is visible
func (e *EnvSelector) Visible() bool {
	return e.visible
}

// SetSize updates the popup dimensions
func (e *EnvSelector) SetSize(width, height int) {
	e.width = width
	e.height = height
}

// SetOnSelect sets the callback for when an environment is selected
func (e *EnvSelector) SetOnSelect(fn func(w *worker.Worker, env string)) {
	e.onSelect = fn
}

// SelectedEnv returns the currently highlighted environment
func (e *EnvSelector) SelectedEnv() string {
	if e.worker == nil || len(e.worker.Environments) == 0 {
		return ""
	}
	if e.cursor >= len(e.worker.Environments) {
		return e.worker.Environments[0]
	}
	return e.worker.Environments[e.cursor]
}

// Update handles key events for the popup
func (e *EnvSelector) Update(msg tea.Msg) tea.Cmd {
	if !e.visible {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			e.Hide()
			return nil

		case "j", "down":
			if e.cursor < len(e.worker.Environments)-1 {
				e.cursor++
			}

		case "k", "up":
			if e.cursor > 0 {
				e.cursor--
			}

		case "g":
			e.cursor = 0

		case "G":
			e.cursor = len(e.worker.Environments) - 1

		case "enter":
			if len(e.worker.Environments) > 0 {
				selectedEnv := e.worker.Environments[e.cursor]
				e.worker.SelectedEnv = selectedEnv
				if e.onSelect != nil {
					e.onSelect(e.worker, selectedEnv)
				}
				e.Hide()
				return nil
			}
		}
	}

	return nil
}

// View renders the popup
func (e *EnvSelector) View() string {
	if !e.visible || e.worker == nil {
		return ""
	}

	t := theme.Current

	// Calculate dimensions
	overlayWidth := min(50, e.width-4)
	overlayHeight := min(len(e.worker.Environments)+8, e.height-4)

	// Header
	header := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Render(fmt.Sprintf("Select Environment • %s", e.worker.Name))

	// Content - environment list
	content := e.renderEnvList(overlayWidth-4, overlayHeight-6)

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(t.Secondary).
		Render("Enter: select • j/k: navigate • Esc: cancel")

	// Overlay style
	overlayStyle := lipgloss.NewStyle().
		Width(overlayWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).
		Padding(1, 2).
		Background(t.Background)

	body := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		content,
		"",
		footer,
	)

	return overlayStyle.Render(body)
}

func (e *EnvSelector) renderEnvList(width, height int) string {
	if e.worker == nil || len(e.worker.Environments) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.Current.Secondary).
			Render("No environments configured")
	}

	t := theme.Current
	var lines []string

	selectedStyle := lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(t.Background).
		Bold(true).
		Width(width)

	normalStyle := lipgloss.NewStyle().
		Foreground(t.Foreground).
		Width(width)

	currentStyle := lipgloss.NewStyle().
		Foreground(t.Success)

	accountStyle := lipgloss.NewStyle().
		Foreground(t.Secondary)

	for i, env := range e.worker.Environments {
		// Get account ID for this environment
		accountID := e.worker.GetAccountIDForEnv(env)
		accountSuffix := ""
		if accountID != "" {
			// Truncate long account IDs
			if len(accountID) > 10 {
				accountID = accountID[:10] + "..."
			}
			accountSuffix = accountStyle.Render(fmt.Sprintf(" (%s)", accountID))
		}

		// Mark current selection
		currentMarker := "  "
		if env == e.worker.SelectedEnv {
			currentMarker = currentStyle.Render("✓ ")
		}

		line := fmt.Sprintf("%s%s%s", currentMarker, env, accountSuffix)

		if i == e.cursor {
			line = fmt.Sprintf("> %s%s", env, accountSuffix)
			lines = append(lines, selectedStyle.Render(line))
		} else {
			lines = append(lines, normalStyle.Render(line))
		}

		if len(lines) >= height {
			break
		}
	}

	return strings.Join(lines, "\n")
}

// EnvSelectedMsg is sent when an environment is selected
type EnvSelectedMsg struct {
	Worker      *worker.Worker
	Environment string
}
