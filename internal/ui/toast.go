package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
)

// ToastType represents the type of toast notification
type ToastType int

const (
	ToastInfo ToastType = iota
	ToastSuccess
	ToastWarning
	ToastError
)

// Toast represents a single toast notification
type Toast struct {
	Message   string
	Type      ToastType
	Duration  time.Duration
	CreatedAt time.Time
}

// ToastManager manages toast notifications
type ToastManager struct {
	toasts      []Toast
	maxToasts   int
	width       int
	defaultDur  time.Duration
}

// NewToastManager creates a new toast manager
func NewToastManager() *ToastManager {
	return &ToastManager{
		toasts:     make([]Toast, 0),
		maxToasts:  5,
		defaultDur: 3 * time.Second,
	}
}

// SetWidth sets the toast width
func (tm *ToastManager) SetWidth(w int) {
	tm.width = w
}

// Add adds a new toast notification
func (tm *ToastManager) Add(message string, toastType ToastType) {
	toast := Toast{
		Message:   message,
		Type:      toastType,
		Duration:  tm.defaultDur,
		CreatedAt: time.Now(),
	}

	tm.toasts = append(tm.toasts, toast)

	// Limit number of toasts
	if len(tm.toasts) > tm.maxToasts {
		tm.toasts = tm.toasts[1:]
	}
}

// AddWithDuration adds a toast with custom duration
func (tm *ToastManager) AddWithDuration(message string, toastType ToastType, duration time.Duration) {
	toast := Toast{
		Message:   message,
		Type:      toastType,
		Duration:  duration,
		CreatedAt: time.Now(),
	}

	tm.toasts = append(tm.toasts, toast)

	if len(tm.toasts) > tm.maxToasts {
		tm.toasts = tm.toasts[1:]
	}
}

// Info adds an info toast
func (tm *ToastManager) Info(message string) {
	tm.Add(message, ToastInfo)
}

// Success adds a success toast
func (tm *ToastManager) Success(message string) {
	tm.Add(message, ToastSuccess)
}

// Warning adds a warning toast
func (tm *ToastManager) Warning(message string) {
	tm.Add(message, ToastWarning)
}

// Error adds an error toast
func (tm *ToastManager) Error(message string) {
	tm.Add(message, ToastError)
}

// Update removes expired toasts
func (tm *ToastManager) Update() {
	now := time.Now()
	active := make([]Toast, 0)

	for _, toast := range tm.toasts {
		if now.Sub(toast.CreatedAt) < toast.Duration {
			active = append(active, toast)
		}
	}

	tm.toasts = active
}

// HasToasts returns true if there are active toasts
func (tm *ToastManager) HasToasts() bool {
	return len(tm.toasts) > 0
}

// Clear removes all toasts
func (tm *ToastManager) Clear() {
	tm.toasts = nil
}

// TickCmd returns a command to update toasts
func (tm *ToastManager) TickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return ToastTickMsg{}
	})
}

// ToastTickMsg is sent to update toast visibility
type ToastTickMsg struct{}

// View renders all active toasts
func (tm *ToastManager) View() string {
	if len(tm.toasts) == 0 {
		return ""
	}

	t := theme.Current
	toastWidth := 40
	if tm.width > 0 && tm.width-10 < toastWidth {
		toastWidth = tm.width - 10
	}

	var rendered []string

	for _, toast := range tm.toasts {
		var style lipgloss.Style
		var icon string

		switch toast.Type {
		case ToastSuccess:
			style = lipgloss.NewStyle().
				Background(t.Success).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1).
				Width(toastWidth)
			icon = "✓ "
		case ToastWarning:
			style = lipgloss.NewStyle().
				Background(t.Warning).
				Foreground(lipgloss.Color("0")).
				Padding(0, 1).
				Width(toastWidth)
			icon = "⚠ "
		case ToastError:
			style = lipgloss.NewStyle().
				Background(t.Error).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1).
				Width(toastWidth)
			icon = "✗ "
		default: // Info
			style = lipgloss.NewStyle().
				Background(t.Primary).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1).
				Width(toastWidth)
			icon = "ℹ "
		}

		// Calculate remaining time for fade effect
		elapsed := time.Since(toast.CreatedAt)
		remaining := toast.Duration - elapsed

		// Add opacity indicator for fading toasts
		message := icon + toast.Message
		if remaining < 500*time.Millisecond {
			// Fade effect - make text dimmer
			style = style.Faint(true)
		}

		rendered = append(rendered, style.Render(message))
	}

	// Stack toasts vertically
	container := lipgloss.NewStyle().
		Padding(0, 1)

	return container.Render(lipgloss.JoinVertical(lipgloss.Right, rendered...))
}

// RenderPositioned renders toasts at a specific position
func (tm *ToastManager) RenderPositioned(screenWidth, screenHeight int) string {
	if len(tm.toasts) == 0 {
		return ""
	}

	toastView := tm.View()

	// Position in top-right corner
	positionStyle := lipgloss.NewStyle().
		Width(screenWidth).
		Align(lipgloss.Right).
		Padding(1, 2)

	return positionStyle.Render(toastView)
}
