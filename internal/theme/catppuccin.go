package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Catppuccin is a theme based on the Catppuccin color palette (Mocha variant)
// https://github.com/catppuccin/catppuccin
var Catppuccin = Theme{
	Name: "Catppuccin",

	// Primary colors - Mauve (Purple-Pink)
	Primary: lipgloss.AdaptiveColor{
		Light: "#8839ef", // Catppuccin Latte Mauve
		Dark:  "#cba6f7", // Catppuccin Mocha Mauve
	},
	PrimaryFaint: lipgloss.AdaptiveColor{
		Light: "#9c5bef",
		Dark:  "#d4bbf7",
	},

	// Secondary colors - Sapphire (Blue)
	Secondary: lipgloss.AdaptiveColor{
		Light: "#209fb5", // Catppuccin Latte Sapphire
		Dark:  "#74c7ec", // Catppuccin Mocha Sapphire
	},
	SecondaryFaint: lipgloss.AdaptiveColor{
		Light: "#45b3c3",
		Dark:  "#8dd1f0",
	},

	// Semantic colors (Mocha palette)
	Success: lipgloss.Color("#a6e3a1"), // Catppuccin Mocha Green
	Warning: lipgloss.Color("#f9e2af"), // Catppuccin Mocha Yellow
	Error:   lipgloss.Color("#f38ba8"), // Catppuccin Mocha Red
	Muted:   lipgloss.Color("#6c7086"), // Catppuccin Mocha Overlay0

	// Background/Foreground (Mocha palette)
	Background: lipgloss.AdaptiveColor{
		Light: "#eff1f5", // Catppuccin Latte Base
		Dark:  "#1e1e2e", // Catppuccin Mocha Base
	},
	BackgroundLight: lipgloss.AdaptiveColor{
		Light: "#e6e9ef", // Catppuccin Latte Mantle
		Dark:  "#313244", // Catppuccin Mocha Surface0
	},
	Foreground: lipgloss.AdaptiveColor{
		Light: "#4c4f69", // Catppuccin Latte Text
		Dark:  "#cdd6f4", // Catppuccin Mocha Text
	},
	ForegroundMuted: lipgloss.AdaptiveColor{
		Light: "#6c6f85", // Catppuccin Latte Subtext0
		Dark:  "#a6adc8", // Catppuccin Mocha Subtext0
	},

	// Border colors
	Border:        lipgloss.Color("#45475a"), // Catppuccin Mocha Surface1
	BorderFocused: lipgloss.Color("#cba6f7"), // Catppuccin Mocha Mauve
	BorderMuted:   lipgloss.Color("#313244"), // Catppuccin Mocha Surface0

	// Special colors
	Highlight:        lipgloss.Color("#f9e2af"), // Catppuccin Mocha Yellow
	Selection:        lipgloss.Color("#45475a"), // Catppuccin Mocha Surface1
	CurrentLine:      lipgloss.Color("#313244"), // Catppuccin Mocha Surface0
	LineNumber:       lipgloss.Color("#6c7086"), // Catppuccin Mocha Overlay0
	LineNumberActive: lipgloss.Color("#cdd6f4"), // Catppuccin Mocha Text

	// Debug specific
	BreakpointActive:   lipgloss.Color("#f38ba8"), // Catppuccin Mocha Red
	BreakpointInactive: lipgloss.Color("#6c7086"), // Catppuccin Mocha Overlay0
	CurrentExecution:   lipgloss.Color("#fab387"), // Catppuccin Mocha Peach

	// VI Mode
	ViNormal:  lipgloss.Color("#89b4fa"), // Catppuccin Mocha Blue
	ViInsert:  lipgloss.Color("#a6e3a1"), // Catppuccin Mocha Green
	ViReplace: lipgloss.Color("#fab387"), // Catppuccin Mocha Peach
}
