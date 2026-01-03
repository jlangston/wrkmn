package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/debug"
	"wrkmn/internal/theme"
)

// VariableNode represents a variable in the tree
type VariableNode struct {
	Name       string
	Value      string
	Type       string
	ObjectID   string // For expandable objects
	Expanded   bool
	Children   []VariableNode
	Depth      int
	HasChildren bool
}

// VariablesView displays variables and scopes
type VariablesView struct {
	session   *debug.Session
	evaluator *debug.Evaluator

	// Current state
	nodes        []VariableNode
	flatNodes    []VariableNode // Flattened for display
	selectedIdx  int
	scrollOffset int

	// Dimensions
	width  int
	height int

	// Focus
	focused bool
}

// NewVariablesView creates a new variables view
func NewVariablesView(session *debug.Session, evaluator *debug.Evaluator) *VariablesView {
	return &VariablesView{
		session:   session,
		evaluator: evaluator,
	}
}

// SetSize sets the view dimensions
func (v *VariablesView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// SetFocus sets whether the view is focused
func (v *VariablesView) SetFocus(focused bool) {
	v.focused = focused
}

// Refresh loads variables from the current call frame
func (v *VariablesView) Refresh() {
	stack := v.session.State().GetCallStack()
	if len(stack) == 0 {
		v.nodes = nil
		v.flatNodes = nil
		return
	}

	frame := stack[0]
	v.nodes = v.loadScopes(frame.ScopeChain)
	v.flattenNodes()
}

// RefreshForFrame loads variables for a specific frame
func (v *VariablesView) RefreshForFrame(frame debug.CallFrame) {
	v.nodes = v.loadScopes(frame.ScopeChain)
	v.flattenNodes()
}

// loadScopes loads variables from scope chain
func (v *VariablesView) loadScopes(scopes []debug.Scope) []VariableNode {
	var nodes []VariableNode

	for _, scope := range scopes {
		// Skip global scope (too many items)
		if scope.Type == "global" {
			continue
		}

		scopeName := scope.Type
		if scope.Name != "" {
			scopeName = scope.Name
		}

		scopeNode := VariableNode{
			Name:        scopeName,
			Value:       "",
			Type:        "scope",
			ObjectID:    scope.Object.ObjectID,
			Expanded:    scope.Type == "local", // Auto-expand local scope
			HasChildren: true,
			Depth:       0,
		}

		// Load properties for expanded scopes
		if scopeNode.Expanded && scopeNode.ObjectID != "" {
			scopeNode.Children = v.loadProperties(scopeNode.ObjectID, 1)
		}

		nodes = append(nodes, scopeNode)
	}

	return nodes
}

// loadProperties loads properties for an object
func (v *VariablesView) loadProperties(objectID string, depth int) []VariableNode {
	props, err := v.evaluator.GetProperties(objectID, true)
	if err != nil {
		return nil
	}

	var nodes []VariableNode
	for _, prop := range props {
		node := VariableNode{
			Name:  prop.Name,
			Value: v.formatValue(prop.Value),
			Type:  prop.Value.Type,
			Depth: depth,
		}

		// Check if expandable
		if prop.Value.ObjectID != "" && prop.Value.Type == "object" {
			node.ObjectID = prop.Value.ObjectID
			node.HasChildren = true
		}

		nodes = append(nodes, node)
	}

	return nodes
}

// formatValue formats a remote object value for display
func (v *VariablesView) formatValue(obj debug.RemoteObject) string {
	switch obj.Type {
	case "undefined":
		return "undefined"
	case "null":
		return "null"
	case "boolean", "number":
		return fmt.Sprintf("%v", obj.Value)
	case "string":
		s := fmt.Sprintf("%v", obj.Value)
		if len(s) > 50 {
			s = s[:47] + "..."
		}
		return fmt.Sprintf("%q", s)
	case "symbol", "bigint":
		return obj.Description
	case "function":
		return "ƒ()"
	case "object":
		if obj.Subtype == "array" {
			return obj.Description
		}
		if obj.ClassName != "" {
			return obj.ClassName + " {...}"
		}
		return "{...}"
	default:
		if obj.Description != "" {
			return obj.Description
		}
		return fmt.Sprintf("[%s]", obj.Type)
	}
}

// flattenNodes flattens the tree for display
func (v *VariablesView) flattenNodes() {
	v.flatNodes = nil
	v.flattenRecursive(v.nodes)
}

func (v *VariablesView) flattenRecursive(nodes []VariableNode) {
	for i := range nodes {
		v.flatNodes = append(v.flatNodes, nodes[i])
		if nodes[i].Expanded && len(nodes[i].Children) > 0 {
			v.flattenRecursive(nodes[i].Children)
		}
	}
}

// Update handles key events
func (v *VariablesView) Update(msg tea.Msg) tea.Cmd {
	if !v.focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if v.selectedIdx < len(v.flatNodes)-1 {
				v.selectedIdx++
				v.ensureVisible()
			}
		case "k", "up":
			if v.selectedIdx > 0 {
				v.selectedIdx--
				v.ensureVisible()
			}
		case "enter", "l", "right":
			v.toggleExpand()
		case "h", "left":
			v.collapse()
		case "g":
			v.selectedIdx = 0
			v.scrollOffset = 0
		case "G":
			v.selectedIdx = len(v.flatNodes) - 1
			v.ensureVisible()
		}
	}

	return nil
}

// toggleExpand expands or collapses the selected node
func (v *VariablesView) toggleExpand() {
	if v.selectedIdx >= len(v.flatNodes) {
		return
	}

	node := &v.flatNodes[v.selectedIdx]
	if !node.HasChildren {
		return
	}

	// Find and update the actual node in the tree
	v.toggleNodeInTree(v.nodes, node.Name, node.Depth)
	v.flattenNodes()
}

// toggleNodeInTree finds and toggles a node in the tree
func (v *VariablesView) toggleNodeInTree(nodes []VariableNode, name string, depth int) bool {
	for i := range nodes {
		if nodes[i].Name == name && nodes[i].Depth == depth {
			nodes[i].Expanded = !nodes[i].Expanded
			if nodes[i].Expanded && len(nodes[i].Children) == 0 && nodes[i].ObjectID != "" {
				nodes[i].Children = v.loadProperties(nodes[i].ObjectID, nodes[i].Depth+1)
			}
			return true
		}
		if v.toggleNodeInTree(nodes[i].Children, name, depth) {
			return true
		}
	}
	return false
}

// collapse collapses the selected node or goes to parent
func (v *VariablesView) collapse() {
	if v.selectedIdx >= len(v.flatNodes) {
		return
	}

	node := &v.flatNodes[v.selectedIdx]
	if node.Expanded {
		v.toggleNodeInTree(v.nodes, node.Name, node.Depth)
		v.flattenNodes()
	}
}

// ensureVisible ensures the selected item is visible
func (v *VariablesView) ensureVisible() {
	visibleLines := v.height - 4
	if visibleLines < 1 {
		visibleLines = 1
	}

	if v.selectedIdx < v.scrollOffset {
		v.scrollOffset = v.selectedIdx
	} else if v.selectedIdx >= v.scrollOffset+visibleLines {
		v.scrollOffset = v.selectedIdx - visibleLines + 1
	}
}

// View renders the variables view
func (v *VariablesView) View() string {
	t := theme.Current
	styles := theme.NewStyles(t)

	// Header
	header := styles.Title.Render("Variables")

	if len(v.flatNodes) == 0 {
		return v.renderContainer(header, styles.Muted.Render("No variables"))
	}

	// Calculate visible items
	visibleLines := v.height - 4
	if visibleLines < 1 {
		visibleLines = 1
	}

	endIdx := v.scrollOffset + visibleLines
	if endIdx > len(v.flatNodes) {
		endIdx = len(v.flatNodes)
	}

	// Styles
	selectedStyle := lipgloss.NewStyle().
		Background(t.Selection).
		Foreground(lipgloss.Color("230")).
		Bold(true)

	nameStyle := lipgloss.NewStyle().
		Foreground(t.Secondary)

	valueStyle := lipgloss.NewStyle().
		Foreground(t.Foreground)

	typeStyle := lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	expandIcon := lipgloss.NewStyle().
		Foreground(t.Primary)

	// Render items
	var lines []string
	for i := v.scrollOffset; i < endIdx; i++ {
		node := v.flatNodes[i]

		// Indent
		indent := strings.Repeat("  ", node.Depth)

		// Expand/collapse icon
		icon := "  "
		if node.HasChildren {
			if node.Expanded {
				icon = expandIcon.Render("▼ ")
			} else {
				icon = expandIcon.Render("▶ ")
			}
		}

		// Build line
		var line string
		if node.Type == "scope" {
			// Scope header
			line = indent + icon + nameStyle.Render(node.Name)
		} else {
			// Variable
			name := nameStyle.Render(node.Name)
			value := valueStyle.Render(node.Value)
			line = indent + icon + name + ": " + value
		}

		// Truncate if needed
		maxWidth := v.width - 6
		if len(line) > maxWidth {
			line = line[:maxWidth-3] + "..."
		}

		// Apply selection
		if i == v.selectedIdx && v.focused {
			line = selectedStyle.Width(v.width - 4).Render(line)
		} else {
			line = typeStyle.Width(v.width - 4).Render(line)
		}

		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return v.renderContainer(header, content)
}

// renderContainer renders the view container
func (v *VariablesView) renderContainer(header, content string) string {
	t := theme.Current

	borderColor := t.Border
	if v.focused {
		borderColor = t.BorderFocused
	}

	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(v.width).
		Height(v.height).
		Padding(0, 1)

	return container.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", content))
}
