package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/config"
	"wrkmn/internal/theme"
	"wrkmn/internal/worker"
)

type devModeOption struct {
	mode  config.DevMode
	label string
	desc  string
}

var devModeOptions = []devModeOption{
	{config.DevModeLocal, "local", "All bindings local (cf dev --local)."},
	{config.DevModeRemoteBindings, "remote-bindings", "Code local; remote bindings honored (plain cf dev). Default."},
}

// ModeSelector displays the available dev modes for a worker.
type ModeSelector struct {
	visible bool
	worker  *worker.Worker
	cursor  int
	width   int
	height  int
}

func NewModeSelector() *ModeSelector { return &ModeSelector{} }

func (s *ModeSelector) Show(w *worker.Worker) {
	if w == nil {
		return
	}
	s.visible = true
	s.worker = w
	s.cursor = 0
	for i, opt := range devModeOptions {
		if opt.mode == w.EffectiveDevMode() {
			s.cursor = i
			break
		}
	}
}

func (s *ModeSelector) Hide()            { s.visible = false }
func (s *ModeSelector) Visible() bool    { return s.visible }
func (s *ModeSelector) SetSize(w, h int) { s.width, s.height = w, h }

func (s *ModeSelector) Update(msg tea.Msg) tea.Cmd {
	if !s.visible {
		return nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "esc":
			s.Hide()
		case "j", "down":
			if s.cursor < len(devModeOptions)-1 {
				s.cursor++
			}
		case "k", "up":
			if s.cursor > 0 {
				s.cursor--
			}
		case "enter":
			s.worker.DevMode = devModeOptions[s.cursor].mode
			s.Hide()
		}
	}
	return nil
}

func (s *ModeSelector) View() string {
	if !s.visible || s.worker == nil {
		return ""
	}
	t := theme.Current
	overlayWidth := min(60, s.width-4)

	header := lipgloss.NewStyle().Foreground(t.Primary).Bold(true).
		Render(fmt.Sprintf("Select Dev Mode • %s", s.worker.Name))

	var lines []string
	selectedStyle := lipgloss.NewStyle().Background(t.Primary).Foreground(t.Background).Bold(true).Width(overlayWidth - 4)
	normalStyle := lipgloss.NewStyle().Foreground(t.Foreground).Width(overlayWidth - 4)
	descStyle := lipgloss.NewStyle().Foreground(t.Secondary)

	for i, opt := range devModeOptions {
		marker := "  "
		if opt.mode == s.worker.EffectiveDevMode() {
			marker = "✓ "
		}
		title := fmt.Sprintf("%s%s", marker, opt.label)
		if i == s.cursor {
			title = fmt.Sprintf("> %s", opt.label)
			lines = append(lines, selectedStyle.Render(title))
		} else {
			lines = append(lines, normalStyle.Render(title))
		}
		lines = append(lines, descStyle.Render("    "+opt.desc))
	}

	footer := lipgloss.NewStyle().Foreground(t.Secondary).Render("Enter: select • j/k: navigate • Esc: cancel")

	body := lipgloss.JoinVertical(lipgloss.Left, header, "", strings.Join(lines, "\n"), "", footer)
	return lipgloss.NewStyle().Width(overlayWidth).Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).Padding(1, 2).Background(t.Background).Render(body)
}
