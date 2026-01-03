package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Dracula is a theme based on the popular Dracula color scheme
// https://draculatheme.com/contribute
var Dracula = Theme{
	Name: "Dracula",

	// Primary colors - Purple
	Primary: lipgloss.AdaptiveColor{
		Light: "#6272a4", // Dracula Comment for light
		Dark:  "#bd93f9", // Dracula Purple
	},
	PrimaryFaint: lipgloss.AdaptiveColor{
		Light: "#7c8dba",
		Dark:  "#caa9fa",
	},

	// Secondary colors - Pink
	Secondary: lipgloss.AdaptiveColor{
		Light: "#d6619c",
		Dark:  "#ff79c6", // Dracula Pink
	},
	SecondaryFaint: lipgloss.AdaptiveColor{
		Light: "#e087b3",
		Dark:  "#ff92d0",
	},

	// Semantic colors
	Success: lipgloss.Color("#50fa7b"), // Dracula Green
	Warning: lipgloss.Color("#f1fa8c"), // Dracula Yellow
	Error:   lipgloss.Color("#ff5555"), // Dracula Red
	Muted:   lipgloss.Color("#6272a4"), // Dracula Comment

	// Background/Foreground
	Background: lipgloss.AdaptiveColor{
		Light: "#f8f8f2", // Dracula Foreground (inverted for light)
		Dark:  "#282a36", // Dracula Background
	},
	BackgroundLight: lipgloss.AdaptiveColor{
		Light: "#e8e8e2",
		Dark:  "#44475a", // Dracula Current Line
	},
	Foreground: lipgloss.AdaptiveColor{
		Light: "#282a36", // Dracula Background (inverted for light)
		Dark:  "#f8f8f2", // Dracula Foreground
	},
	ForegroundMuted: lipgloss.AdaptiveColor{
		Light: "#44475a",
		Dark:  "#6272a4", // Dracula Comment
	},

	// Border colors
	Border:        lipgloss.Color("#44475a"), // Dracula Current Line
	BorderFocused: lipgloss.Color("#bd93f9"), // Dracula Purple
	BorderMuted:   lipgloss.Color("#383a46"),

	// Special colors
	Highlight:        lipgloss.Color("#f1fa8c"), // Dracula Yellow
	Selection:        lipgloss.Color("#44475a"), // Dracula Current Line
	CurrentLine:      lipgloss.Color("#44475a"), // Dracula Current Line
	LineNumber:       lipgloss.Color("#6272a4"), // Dracula Comment
	LineNumberActive: lipgloss.Color("#f8f8f2"), // Dracula Foreground

	// Debug specific
	BreakpointActive:   lipgloss.Color("#ff5555"), // Dracula Red
	BreakpointInactive: lipgloss.Color("#6272a4"), // Dracula Comment
	CurrentExecution:   lipgloss.Color("#ffb86c"), // Dracula Orange

	// VI Mode
	ViNormal:  lipgloss.Color("#bd93f9"), // Dracula Purple
	ViInsert:  lipgloss.Color("#50fa7b"), // Dracula Green
	ViReplace: lipgloss.Color("#ffb86c"), // Dracula Orange
}
