package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Default is the default theme with a cool blue/teal color scheme
var Default = Theme{
	Name: "Default",

	// Primary colors - Teal/Cyan
	Primary: lipgloss.AdaptiveColor{
		Light: "#0891b2", // cyan-600
		Dark:  "#22d3ee", // cyan-400
	},
	PrimaryFaint: lipgloss.AdaptiveColor{
		Light: "#06b6d4", // cyan-500
		Dark:  "#67e8f9", // cyan-300
	},

	// Secondary colors - Indigo
	Secondary: lipgloss.AdaptiveColor{
		Light: "#4f46e5", // indigo-600
		Dark:  "#818cf8", // indigo-400
	},
	SecondaryFaint: lipgloss.AdaptiveColor{
		Light: "#6366f1", // indigo-500
		Dark:  "#a5b4fc", // indigo-300
	},

	// Semantic colors
	Success: lipgloss.Color("#22c55e"), // green-500
	Warning: lipgloss.Color("#eab308"), // yellow-500
	Error:   lipgloss.Color("#ef4444"), // red-500
	Muted:   lipgloss.Color("#6b7280"), // gray-500

	// Background/Foreground
	Background: lipgloss.AdaptiveColor{
		Light: "#ffffff",
		Dark:  "#0f172a", // slate-900
	},
	BackgroundLight: lipgloss.AdaptiveColor{
		Light: "#f8fafc", // slate-50
		Dark:  "#1e293b", // slate-800
	},
	Foreground: lipgloss.AdaptiveColor{
		Light: "#0f172a", // slate-900
		Dark:  "#f8fafc", // slate-50
	},
	ForegroundMuted: lipgloss.AdaptiveColor{
		Light: "#64748b", // slate-500
		Dark:  "#94a3b8", // slate-400
	},

	// Border colors
	Border:        lipgloss.Color("#334155"), // slate-700
	BorderFocused: lipgloss.Color("#22d3ee"), // cyan-400
	BorderMuted:   lipgloss.Color("#475569"), // slate-600

	// Special colors
	Highlight:        lipgloss.Color("#fbbf24"), // amber-400
	Selection:        lipgloss.Color("#0891b2"), // cyan-600
	CurrentLine:      lipgloss.Color("#1e3a5f"), // custom dark blue
	LineNumber:       lipgloss.Color("#64748b"), // slate-500
	LineNumberActive: lipgloss.Color("#e2e8f0"), // slate-200

	// Debug specific
	BreakpointActive:   lipgloss.Color("#ef4444"), // red-500
	BreakpointInactive: lipgloss.Color("#6b7280"), // gray-500
	CurrentExecution:   lipgloss.Color("#fbbf24"), // amber-400

	// VI Mode
	ViNormal:  lipgloss.Color("#0891b2"), // cyan-600
	ViInsert:  lipgloss.Color("#22c55e"), // green-500
	ViReplace: lipgloss.Color("#f97316"), // orange-500
}
