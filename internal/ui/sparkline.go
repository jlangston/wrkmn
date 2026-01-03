package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
)

// Sparkline characters from lowest to highest
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline represents a sparkline chart
type Sparkline struct {
	values   []float64
	maxLen   int
	min      float64
	max      float64
	autoScale bool
	label    string
	unit     string
	width    int
	color    lipgloss.TerminalColor
}

// NewSparkline creates a new sparkline
func NewSparkline(maxLen int) *Sparkline {
	return &Sparkline{
		values:    make([]float64, 0, maxLen),
		maxLen:    maxLen,
		autoScale: true,
		min:       0,
		max:       100,
		color:     theme.Current.Success, // Use a non-adaptive color
	}
}

// SetLabel sets the sparkline label
func (s *Sparkline) SetLabel(label string) {
	s.label = label
}

// SetUnit sets the unit suffix (e.g., "%", "MB")
func (s *Sparkline) SetUnit(unit string) {
	s.unit = unit
}

// SetWidth sets the display width
func (s *Sparkline) SetWidth(w int) {
	s.width = w
}

// SetColor sets the sparkline color
func (s *Sparkline) SetColor(c lipgloss.TerminalColor) {
	s.color = c
}

// SetRange sets the min/max range and disables auto-scaling
func (s *Sparkline) SetRange(min, max float64) {
	s.min = min
	s.max = max
	s.autoScale = false
}

// Push adds a new value to the sparkline
func (s *Sparkline) Push(value float64) {
	s.values = append(s.values, value)

	// Trim to max length
	if len(s.values) > s.maxLen {
		s.values = s.values[len(s.values)-s.maxLen:]
	}

	// Update auto-scale range
	if s.autoScale && len(s.values) > 0 {
		s.min = s.values[0]
		s.max = s.values[0]
		for _, v := range s.values {
			if v < s.min {
				s.min = v
			}
			if v > s.max {
				s.max = v
			}
		}
		// Add padding to range
		if s.min == s.max {
			s.max = s.min + 1
		}
	}
}

// Current returns the most recent value
func (s *Sparkline) Current() float64 {
	if len(s.values) == 0 {
		return 0
	}
	return s.values[len(s.values)-1]
}

// Average returns the average of all values
func (s *Sparkline) Average() float64 {
	if len(s.values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range s.values {
		sum += v
	}
	return sum / float64(len(s.values))
}

// Render renders the sparkline
func (s *Sparkline) Render() string {
	if len(s.values) == 0 {
		return strings.Repeat(" ", s.width)
	}

	// Calculate display width
	displayWidth := s.width
	if displayWidth == 0 {
		displayWidth = s.maxLen
	}

	// Get values to display
	values := s.values
	if len(values) > displayWidth {
		values = values[len(values)-displayWidth:]
	}

	// Normalize and render
	var chars []rune
	valueRange := s.max - s.min
	if valueRange == 0 {
		valueRange = 1
	}

	for _, v := range values {
		normalized := (v - s.min) / valueRange
		if normalized < 0 {
			normalized = 0
		}
		if normalized > 1 {
			normalized = 1
		}

		idx := int(normalized * float64(len(sparkChars)-1))
		chars = append(chars, sparkChars[idx])
	}

	// Pad to width if needed
	for len(chars) < displayWidth {
		chars = append([]rune{' '}, chars...)
	}

	style := lipgloss.NewStyle().Foreground(s.color)
	return style.Render(string(chars))
}

// View renders the sparkline with label and value
func (s *Sparkline) View() string {
	t := theme.Current

	labelStyle := lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	valueStyle := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	sparkline := s.Render()
	current := fmt.Sprintf("%.1f%s", s.Current(), s.unit)

	if s.label != "" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			labelStyle.Render(s.label),
			sparkline,
			valueStyle.Render(current),
		)
	}

	return sparkline + " " + valueStyle.Render(current)
}

// SparklineGroup manages multiple sparklines
type SparklineGroup struct {
	sparklines map[string]*Sparkline
	order      []string
	width      int
	height     int
}

// NewSparklineGroup creates a new sparkline group
func NewSparklineGroup() *SparklineGroup {
	return &SparklineGroup{
		sparklines: make(map[string]*Sparkline),
		order:      make([]string, 0),
	}
}

// Add adds a new sparkline to the group
func (g *SparklineGroup) Add(name string, maxLen int) *Sparkline {
	s := NewSparkline(maxLen)
	s.SetLabel(name)
	g.sparklines[name] = s
	g.order = append(g.order, name)
	return s
}

// Get returns a sparkline by name
func (g *SparklineGroup) Get(name string) *Sparkline {
	return g.sparklines[name]
}

// Push pushes a value to a named sparkline
func (g *SparklineGroup) Push(name string, value float64) {
	if s, ok := g.sparklines[name]; ok {
		s.Push(value)
	}
}

// SetSize sets the group display size
func (g *SparklineGroup) SetSize(width, height int) {
	g.width = width
	g.height = height

	// Distribute width among sparklines
	sparkWidth := width / max(1, len(g.sparklines))
	for _, s := range g.sparklines {
		s.SetWidth(sparkWidth - 2)
	}
}

// View renders all sparklines in the group
func (g *SparklineGroup) View() string {
	t := theme.Current

	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	var views []string
	for _, name := range g.order {
		if s, ok := g.sparklines[name]; ok {
			views = append(views, s.View())
		}
	}

	return container.Render(lipgloss.JoinHorizontal(lipgloss.Top, views...))
}

// Helper
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ProgressBar renders a simple progress bar
type ProgressBar struct {
	value    float64
	max      float64
	width    int
	label    string
	showPct  bool
	color    lipgloss.TerminalColor
	bgColor  lipgloss.TerminalColor
}

// NewProgressBar creates a new progress bar
func NewProgressBar(width int) *ProgressBar {
	t := theme.Current
	return &ProgressBar{
		max:     100,
		width:   width,
		showPct: true,
		color:   t.Success,
		bgColor: t.Border,
	}
}

// SetValue sets the progress value
func (p *ProgressBar) SetValue(v float64) {
	p.value = v
}

// SetMax sets the maximum value
func (p *ProgressBar) SetMax(m float64) {
	p.max = m
}

// SetLabel sets the label
func (p *ProgressBar) SetLabel(l string) {
	p.label = l
}

// SetColor sets the progress color
func (p *ProgressBar) SetColor(c lipgloss.TerminalColor) {
	p.color = c
}

// View renders the progress bar
func (p *ProgressBar) View() string {
	t := theme.Current

	pct := p.value / p.max
	if pct > 1 {
		pct = 1
	}
	if pct < 0 {
		pct = 0
	}

	filled := int(float64(p.width) * pct)
	empty := p.width - filled

	filledStyle := lipgloss.NewStyle().
		Background(p.color).
		Foreground(lipgloss.Color("230"))

	emptyStyle := lipgloss.NewStyle().
		Background(p.bgColor)

	bar := filledStyle.Render(strings.Repeat(" ", filled)) +
		emptyStyle.Render(strings.Repeat(" ", empty))

	result := bar
	if p.showPct {
		pctText := fmt.Sprintf(" %d%%", int(pct*100))
		pctStyle := lipgloss.NewStyle().Foreground(t.ForegroundMuted)
		result += pctStyle.Render(pctText)
	}

	if p.label != "" {
		labelStyle := lipgloss.NewStyle().Foreground(t.ForegroundMuted)
		result = labelStyle.Render(p.label+" ") + result
	}

	return result
}
