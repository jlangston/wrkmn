package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Nord is a theme based on the Nord color palette
// https://www.nordtheme.com/docs/colors-and-palettes
var Nord = Theme{
	Name: "Nord",

	// Primary colors - Nord8 Frost (Teal)
	Primary: lipgloss.AdaptiveColor{
		Light: "#5e81ac", // Nord10 for light
		Dark:  "#88c0d0", // Nord8 Frost
	},
	PrimaryFaint: lipgloss.AdaptiveColor{
		Light: "#7a9ac2",
		Dark:  "#9dcde0",
	},

	// Secondary colors - Nord9 Frost (Light Blue)
	Secondary: lipgloss.AdaptiveColor{
		Light: "#5e81ac", // Nord10
		Dark:  "#81a1c1", // Nord9
	},
	SecondaryFaint: lipgloss.AdaptiveColor{
		Light: "#7a9ac2",
		Dark:  "#97b5d0",
	},

	// Semantic colors
	Success: lipgloss.Color("#a3be8c"), // Nord14 Aurora Green
	Warning: lipgloss.Color("#ebcb8b"), // Nord13 Aurora Yellow
	Error:   lipgloss.Color("#bf616a"), // Nord11 Aurora Red
	Muted:   lipgloss.Color("#4c566a"), // Nord3

	// Background/Foreground (Polar Night)
	Background: lipgloss.AdaptiveColor{
		Light: "#eceff4", // Nord6 Snow Storm
		Dark:  "#2e3440", // Nord0 Polar Night
	},
	BackgroundLight: lipgloss.AdaptiveColor{
		Light: "#e5e9f0", // Nord5
		Dark:  "#3b4252", // Nord1
	},
	Foreground: lipgloss.AdaptiveColor{
		Light: "#2e3440", // Nord0
		Dark:  "#eceff4", // Nord6 Snow Storm
	},
	ForegroundMuted: lipgloss.AdaptiveColor{
		Light: "#4c566a", // Nord3
		Dark:  "#d8dee9", // Nord4
	},

	// Border colors
	Border:        lipgloss.Color("#4c566a"), // Nord3
	BorderFocused: lipgloss.Color("#88c0d0"), // Nord8
	BorderMuted:   lipgloss.Color("#3b4252"), // Nord1

	// Special colors
	Highlight:        lipgloss.Color("#ebcb8b"), // Nord13 Aurora Yellow
	Selection:        lipgloss.Color("#434c5e"), // Nord2
	CurrentLine:      lipgloss.Color("#3b4252"), // Nord1
	LineNumber:       lipgloss.Color("#4c566a"), // Nord3
	LineNumberActive: lipgloss.Color("#eceff4"), // Nord6

	// Debug specific
	BreakpointActive:   lipgloss.Color("#bf616a"), // Nord11 Aurora Red
	BreakpointInactive: lipgloss.Color("#4c566a"), // Nord3
	CurrentExecution:   lipgloss.Color("#d08770"), // Nord12 Aurora Orange

	// VI Mode
	ViNormal:  lipgloss.Color("#5e81ac"), // Nord10
	ViInsert:  lipgloss.Color("#a3be8c"), // Nord14 Green
	ViReplace: lipgloss.Color("#d08770"), // Nord12 Orange
}
