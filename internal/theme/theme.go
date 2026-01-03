package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the color scheme and styles for the application
type Theme struct {
	Name string

	// Transparency support
	TransparentBackground bool // Use terminal's background instead of theme background

	// Primary colors
	Primary       lipgloss.AdaptiveColor
	PrimaryFaint  lipgloss.AdaptiveColor
	Secondary     lipgloss.AdaptiveColor
	SecondaryFaint lipgloss.AdaptiveColor

	// Semantic colors
	Success lipgloss.Color // Running, healthy
	Warning lipgloss.Color // Starting, degraded
	Error   lipgloss.Color // Failed, error
	Muted   lipgloss.Color // Disabled, stopped

	// Background/Foreground
	Background      lipgloss.AdaptiveColor
	BackgroundLight lipgloss.AdaptiveColor
	Foreground      lipgloss.AdaptiveColor
	ForegroundMuted lipgloss.AdaptiveColor

	// Border colors
	Border        lipgloss.Color
	BorderFocused lipgloss.Color
	BorderMuted   lipgloss.Color

	// Special colors
	Highlight      lipgloss.Color
	Selection      lipgloss.Color
	CurrentLine    lipgloss.Color
	LineNumber     lipgloss.Color
	LineNumberActive lipgloss.Color

	// Debug specific
	BreakpointActive   lipgloss.Color
	BreakpointInactive lipgloss.Color
	CurrentExecution   lipgloss.Color

	// VI Mode
	ViNormal  lipgloss.Color
	ViInsert  lipgloss.Color
	ViReplace lipgloss.Color
}

// Current holds the currently active theme
var Current = Default

// Styles provides pre-built styles using the current theme
type Styles struct {
	// Base styles
	Base       lipgloss.Style
	Focused    lipgloss.Style
	Blurred    lipgloss.Style

	// Container styles
	Panel      lipgloss.Style
	PanelFocused lipgloss.Style
	Header     lipgloss.Style
	Footer     lipgloss.Style

	// Text styles
	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Label      lipgloss.Style
	Value      lipgloss.Style
	Muted      lipgloss.Style
	Error      lipgloss.Style
	Success    lipgloss.Style
	Warning    lipgloss.Style

	// Table styles
	TableHeader    lipgloss.Style
	TableRowEven   lipgloss.Style
	TableRowOdd    lipgloss.Style
	TableRowSelected lipgloss.Style

	// Status badges
	StatusRunning  lipgloss.Style
	StatusStopped  lipgloss.Style
	StatusFailed   lipgloss.Style
	StatusStarting lipgloss.Style
	StatusHealthy  lipgloss.Style

	// Help styles
	HelpKey   lipgloss.Style
	HelpValue lipgloss.Style

	// Debug styles
	SourceCode     lipgloss.Style
	LineNumber     lipgloss.Style
	CurrentLine    lipgloss.Style
	Breakpoint     lipgloss.Style
	CallFrame      lipgloss.Style
	CallFrameActive lipgloss.Style
	Variable       lipgloss.Style
	VariableValue  lipgloss.Style

	// VI Mode indicators
	ViNormalMode  lipgloss.Style
	ViInsertMode  lipgloss.Style
	ViReplaceMode lipgloss.Style
}

// NewStyles creates a new Styles instance based on the given theme
func NewStyles(t Theme) Styles {
	return Styles{
		// Base styles
		Base: lipgloss.NewStyle().
			Foreground(t.Foreground),
		Focused: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		Blurred: lipgloss.NewStyle().
			Foreground(t.ForegroundMuted),

		// Container styles
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.Border).
			Padding(0, 1),
		PanelFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(t.BorderFocused).
			Padding(0, 1),
		Header: lipgloss.NewStyle().
			Background(t.Primary).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1),
		Footer: lipgloss.NewStyle().
			Foreground(t.ForegroundMuted).
			Padding(0, 1),

		// Text styles
		Title: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		Subtitle: lipgloss.NewStyle().
			Foreground(t.Secondary).
			Italic(true),
		Label: lipgloss.NewStyle().
			Foreground(t.ForegroundMuted),
		Value: lipgloss.NewStyle().
			Foreground(t.Foreground),
		Muted: lipgloss.NewStyle().
			Foreground(t.Muted),
		Error: lipgloss.NewStyle().
			Foreground(t.Error).
			Bold(true),
		Success: lipgloss.NewStyle().
			Foreground(t.Success),
		Warning: lipgloss.NewStyle().
			Foreground(t.Warning),

		// Table styles
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(t.Primary).
			Padding(0, 1),
		TableRowEven: lipgloss.NewStyle().
			Background(t.BackgroundLight).
			Foreground(t.Foreground).
			Padding(0, 1),
		TableRowOdd: lipgloss.NewStyle().
			Foreground(t.Foreground).
			Padding(0, 1),
		TableRowSelected: lipgloss.NewStyle().
			Background(t.Selection).
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Padding(0, 1),

		// Status badges
		StatusRunning: lipgloss.NewStyle().
			Foreground(t.Success).
			Bold(true),
		StatusStopped: lipgloss.NewStyle().
			Foreground(t.Muted),
		StatusFailed: lipgloss.NewStyle().
			Foreground(t.Error).
			Bold(true),
		StatusStarting: lipgloss.NewStyle().
			Foreground(t.Warning).
			Bold(true),
		StatusHealthy: lipgloss.NewStyle().
			Foreground(t.Success),

		// Help styles
		HelpKey: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		HelpValue: lipgloss.NewStyle().
			Foreground(t.ForegroundMuted),

		// Debug styles
		SourceCode: lipgloss.NewStyle().
			Foreground(t.Foreground),
		LineNumber: lipgloss.NewStyle().
			Foreground(t.LineNumber).
			Width(4).
			Align(lipgloss.Right),
		CurrentLine: lipgloss.NewStyle().
			Background(t.CurrentLine).
			Foreground(t.Foreground),
		Breakpoint: lipgloss.NewStyle().
			Foreground(t.BreakpointActive).
			Bold(true),
		CallFrame: lipgloss.NewStyle().
			Foreground(t.ForegroundMuted),
		CallFrameActive: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),
		Variable: lipgloss.NewStyle().
			Foreground(t.Secondary),
		VariableValue: lipgloss.NewStyle().
			Foreground(t.Foreground),

		// VI Mode indicators
		ViNormalMode: lipgloss.NewStyle().
			Background(t.ViNormal).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1),
		ViInsertMode: lipgloss.NewStyle().
			Background(t.ViInsert).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1),
		ViReplaceMode: lipgloss.NewStyle().
			Background(t.ViReplace).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1),
	}
}

// CurrentStyles returns styles for the current theme
func CurrentStyles() Styles {
	return NewStyles(Current)
}

// SetTheme sets the current theme
func SetTheme(t Theme) {
	Current = t
}

// SetTransparent enables or disables transparent background for the current theme
func SetTransparent(transparent bool) {
	Current.TransparentBackground = transparent
}

// WithTransparent returns a copy of the theme with transparent background enabled
func (t Theme) WithTransparent() Theme {
	t.TransparentBackground = true
	return t
}

// BgColor returns the appropriate background color based on transparency setting
// Returns lipgloss.NoColor{} for transparent mode, otherwise the theme background
func (t Theme) BgColor() lipgloss.TerminalColor {
	if t.TransparentBackground {
		return lipgloss.NoColor{}
	}
	return t.Background
}

// BgLightColor returns the appropriate light background color based on transparency setting
// For transparent mode, returns a subtle overlay color instead of solid background
func (t Theme) BgLightColor() lipgloss.TerminalColor {
	if t.TransparentBackground {
		return lipgloss.NoColor{}
	}
	return t.BackgroundLight
}

// Available returns all available themes
func Available() []Theme {
	return []Theme{
		Default,
		Dracula,
		Catppuccin,
		Nord,
	}
}

// GetThemeByName returns a theme by its name
func GetThemeByName(name string) (Theme, bool) {
	for _, t := range Available() {
		if t.Name == name {
			return t, true
		}
	}
	return Default, false
}
