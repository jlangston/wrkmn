package ui

// ViewMode represents the current application view
type ViewMode int

const (
	ViewDashboard   ViewMode = iota // Main process list view
	ViewMonitor                     // 4-panel metrics/status view
	ViewLogs                        // Log viewer for selected process
	ViewRemoteLogs                  // Remote log tailing (CF Workers only)
	ViewDebug                       // Debug view (CF Workers only)
	ViewHelp                        // Help overlay
)

func (v ViewMode) String() string {
	switch v {
	case ViewDashboard:
		return "Dashboard"
	case ViewMonitor:
		return "Monitor"
	case ViewLogs:
		return "Logs"
	case ViewRemoteLogs:
		return "Remote Logs"
	case ViewDebug:
		return "Debug"
	case ViewHelp:
		return "Help"
	default:
		return "Unknown"
	}
}

// ViewStack manages view history for navigation
type ViewStack struct {
	stack []ViewMode
}

// NewViewStack creates a new view stack starting at dashboard
func NewViewStack() *ViewStack {
	return &ViewStack{
		stack: []ViewMode{ViewDashboard},
	}
}

// Current returns the current view
func (vs *ViewStack) Current() ViewMode {
	if len(vs.stack) == 0 {
		return ViewDashboard
	}
	return vs.stack[len(vs.stack)-1]
}

// Push adds a new view to the stack
func (vs *ViewStack) Push(v ViewMode) {
	// Don't push duplicates
	if vs.Current() == v {
		return
	}
	vs.stack = append(vs.stack, v)
}

// Pop returns to the previous view
func (vs *ViewStack) Pop() ViewMode {
	if len(vs.stack) <= 1 {
		return vs.Current()
	}
	vs.stack = vs.stack[:len(vs.stack)-1]
	return vs.Current()
}

// Set sets the current view without pushing (replaces)
func (vs *ViewStack) Set(v ViewMode) {
	if len(vs.stack) == 0 {
		vs.stack = append(vs.stack, v)
	} else {
		vs.stack[len(vs.stack)-1] = v
	}
}

// Clear resets to dashboard only
func (vs *ViewStack) Clear() {
	vs.stack = []ViewMode{ViewDashboard}
}

// CanGoBack returns true if there's a previous view to return to
func (vs *ViewStack) CanGoBack() bool {
	return len(vs.stack) > 1
}

// Breadcrumb returns the view path as a breadcrumb string
func (vs *ViewStack) Breadcrumb() string {
	if len(vs.stack) == 0 {
		return ""
	}
	result := ""
	for i, v := range vs.stack {
		if i > 0 {
			result += " > "
		}
		result += v.String()
	}
	return result
}

// PanelFocus represents which panel has focus in multi-panel views
type PanelFocus int

const (
	FocusMain PanelFocus = iota
	FocusLeft
	FocusRight
	FocusTop
	FocusBottom
)

func (p PanelFocus) String() string {
	switch p {
	case FocusMain:
		return "Main"
	case FocusLeft:
		return "Left"
	case FocusRight:
		return "Right"
	case FocusTop:
		return "Top"
	case FocusBottom:
		return "Bottom"
	default:
		return "Unknown"
	}
}
