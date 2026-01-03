package ui

import (
	"fmt"

	"wrkmn/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds all pre-computed styles for the UI
type Styles struct {
	theme  theme.Theme
	styles theme.Styles

	// Layout styles
	App           lipgloss.Style
	Header        lipgloss.Style
	Footer        lipgloss.Style
	Sidebar       lipgloss.Style
	MainContent   lipgloss.Style

	// Status indicators
	StatusDot     map[string]string
	StatusBadge   map[string]lipgloss.Style

	// Process list
	ProcessRow    lipgloss.Style
	ProcessRowSel lipgloss.Style
	ProcessName   lipgloss.Style
	ProcessPort   lipgloss.Style
	ProcessUptime lipgloss.Style

	// Tabs
	Tab       lipgloss.Style
	TabActive lipgloss.Style

	// Help
	HelpKey   lipgloss.Style
	HelpDesc  lipgloss.Style
	HelpSep   lipgloss.Style

	// Search
	SearchInput    lipgloss.Style
	SearchPrompt   lipgloss.Style
	SearchMatch    lipgloss.Style
	SearchNoMatch  lipgloss.Style

	// VI Mode indicator
	ModeNormal  lipgloss.Style
	ModeInsert  lipgloss.Style
	ModeReplace lipgloss.Style

	// Sparkline
	SparklineContainer lipgloss.Style
	SparklineLabel     lipgloss.Style
	SparklineValue     lipgloss.Style
}

// NewStyles creates a new Styles instance based on the current theme
func NewStyles() *Styles {
	t := theme.Current
	ts := theme.NewStyles(t)

	s := &Styles{
		theme:  t,
		styles: ts,
	}

	// Layout
	s.App = lipgloss.NewStyle().
		Background(t.Background)

	s.Header = lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Padding(0, 1)

	s.Footer = lipgloss.NewStyle().
		Foreground(t.ForegroundMuted).
		Padding(0, 1)

	s.Sidebar = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	s.MainContent = lipgloss.NewStyle().
		Padding(0, 1)

	// Status indicators
	s.StatusDot = map[string]string{
		"running":  lipgloss.NewStyle().Foreground(t.Success).Render("●"),
		"stopped":  lipgloss.NewStyle().Foreground(t.Muted).Render("○"),
		"starting": lipgloss.NewStyle().Foreground(t.Warning).Render("◐"),
		"failed":   lipgloss.NewStyle().Foreground(t.Error).Render("●"),
		"healthy":  lipgloss.NewStyle().Foreground(t.Success).Render("♥"),
		"unhealthy": lipgloss.NewStyle().Foreground(t.Error).Render("✗"),
	}

	s.StatusBadge = map[string]lipgloss.Style{
		"running": lipgloss.NewStyle().
			Background(t.Success).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1).
			Bold(true),
		"stopped": lipgloss.NewStyle().
			Background(t.Muted).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1),
		"starting": lipgloss.NewStyle().
			Background(t.Warning).
			Foreground(lipgloss.Color("0")).
			Padding(0, 1).
			Bold(true),
		"failed": lipgloss.NewStyle().
			Background(t.Error).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1).
			Bold(true),
	}

	// Process list
	s.ProcessRow = lipgloss.NewStyle().
		Foreground(t.Foreground).
		Padding(0, 1)

	s.ProcessRowSel = lipgloss.NewStyle().
		Background(t.Selection).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Padding(0, 1)

	s.ProcessName = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	s.ProcessPort = lipgloss.NewStyle().
		Foreground(t.Secondary)

	s.ProcessUptime = lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	// Tabs
	s.Tab = lipgloss.NewStyle().
		Foreground(t.ForegroundMuted).
		Padding(0, 2)

	s.TabActive = lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Padding(0, 2)

	// Help
	s.HelpKey = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	s.HelpDesc = lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	s.HelpSep = lipgloss.NewStyle().
		Foreground(t.Border)

	// Search
	s.SearchInput = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).
		Padding(0, 1)

	s.SearchPrompt = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	s.SearchMatch = lipgloss.NewStyle().
		Background(t.Highlight).
		Foreground(lipgloss.Color("0"))

	s.SearchNoMatch = lipgloss.NewStyle().
		Foreground(t.Error)

	// VI Mode
	s.ModeNormal = lipgloss.NewStyle().
		Background(t.ViNormal).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Padding(0, 1)

	s.ModeInsert = lipgloss.NewStyle().
		Background(t.ViInsert).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Padding(0, 1)

	s.ModeReplace = lipgloss.NewStyle().
		Background(t.ViReplace).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Padding(0, 1)

	// Sparkline
	s.SparklineContainer = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	s.SparklineLabel = lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	s.SparklineValue = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	return s
}

// GetModeStyle returns the style for a VI mode
func (s *Styles) GetModeStyle(mode string) lipgloss.Style {
	switch mode {
	case "NORMAL":
		return s.ModeNormal
	case "INSERT":
		return s.ModeInsert
	case "REPLACE":
		return s.ModeReplace
	default:
		return s.ModeNormal
	}
}

// GetStatusDot returns the status dot for a status
func (s *Styles) GetStatusDot(status string) string {
	if dot, ok := s.StatusDot[status]; ok {
		return dot
	}
	return s.StatusDot["stopped"]
}

// GetStatusBadge returns the badge style for a status
func (s *Styles) GetStatusBadge(status string) lipgloss.Style {
	if badge, ok := s.StatusBadge[status]; ok {
		return badge
	}
	return s.StatusBadge["stopped"]
}

// FormatUptime formats an uptime duration
func FormatUptime(seconds int64) string {
	if seconds < 60 {
		return "<1m"
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}
