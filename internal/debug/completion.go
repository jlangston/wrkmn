package debug

import (
	"encoding/json"
	"sort"
	"strings"
)

// Completion represents a completion suggestion
type Completion struct {
	Text        string // The completion text to insert
	Label       string // Display label (may include type info)
	Type        string // "variable", "property", "function", "method"
	Description string // Type or value preview
}

// Completer provides autocompletion for the REPL
type Completer struct {
	session   *Session
	evaluator *Evaluator
}

// NewCompleter creates a new completer
func NewCompleter(session *Session, evaluator *Evaluator) *Completer {
	return &Completer{
		session:   session,
		evaluator: evaluator,
	}
}

// GetCompletions returns completions for the given input
func (c *Completer) GetCompletions(input string, cursorPos int) ([]Completion, error) {
	if cursorPos > len(input) {
		cursorPos = len(input)
	}

	// Get the text up to cursor
	text := input[:cursorPos]
	if text == "" {
		return nil, nil
	}

	// Determine completion type based on input
	lastDot := strings.LastIndex(text, ".")
	lastBracket := strings.LastIndex(text, "[")

	if lastDot > lastBracket && lastDot >= 0 {
		// Property completion: "obj.prop" -> complete properties of "obj"
		base := text[:lastDot]
		prefix := text[lastDot+1:]
		return c.getPropertyCompletions(base, prefix)
	}

	// Variable completion
	prefix := getLastIdentifier(text)
	if prefix == "" {
		return nil, nil
	}

	return c.getVariableCompletions(prefix)
}

// getPropertyCompletions returns completions for object properties
func (c *Completer) getPropertyCompletions(baseExpr, prefix string) ([]Completion, error) {
	// Evaluate the base expression to get its objectId
	result, err := c.evaluator.Evaluate(baseExpr)
	if err != nil {
		return nil, err
	}

	if result.ExceptionDetails != nil {
		return nil, nil // Expression threw an error
	}

	if result.Result.ObjectID == "" {
		// Primitive type - no properties to complete
		return nil, nil
	}

	// Get properties of the object
	props, err := c.evaluator.GetProperties(result.Result.ObjectID, false)
	if err != nil {
		return nil, err
	}

	// Filter by prefix and build completions
	var completions []Completion
	prefix = strings.ToLower(prefix)

	for _, prop := range props {
		lowerName := strings.ToLower(prop.Name)
		if prefix == "" || strings.HasPrefix(lowerName, prefix) {
			comp := Completion{
				Text:  prop.Name,
				Label: prop.Name,
			}

			// Determine type
			switch prop.Value.Type {
			case "function":
				comp.Type = "function"
				comp.Label = prop.Name + "()"
				comp.Description = "function"
			default:
				comp.Type = "property"
				comp.Description = formatTypeDescription(prop.Value)
			}

			completions = append(completions, comp)
		}
	}

	// Sort by name
	sort.Slice(completions, func(i, j int) bool {
		return completions[i].Text < completions[j].Text
	})

	// Limit to reasonable number
	if len(completions) > 50 {
		completions = completions[:50]
	}

	return completions, nil
}

// getVariableCompletions returns completions for variable names
func (c *Completer) getVariableCompletions(prefix string) ([]Completion, error) {
	var completions []Completion
	seen := make(map[string]bool)
	prefix = strings.ToLower(prefix)

	// If paused, get scope variables
	if c.session.state.GetState() == StatePaused {
		stack := c.session.state.GetCallStack()
		if len(stack) > 0 {
			// Iterate through scope chain
			for _, scope := range stack[0].ScopeChain {
				if scope.Object.ObjectID == "" {
					continue
				}

				props, err := c.evaluator.GetProperties(scope.Object.ObjectID, true)
				if err != nil {
					continue
				}

				for _, prop := range props {
					lowerName := strings.ToLower(prop.Name)
					if !strings.HasPrefix(lowerName, prefix) {
						continue
					}
					if seen[prop.Name] {
						continue
					}
					seen[prop.Name] = true

					comp := Completion{
						Text:  prop.Name,
						Label: prop.Name,
					}

					switch prop.Value.Type {
					case "function":
						comp.Type = "function"
						comp.Label = prop.Name + "()"
						comp.Description = "function"
					default:
						comp.Type = "variable"
						comp.Description = formatTypeDescription(prop.Value)
					}

					completions = append(completions, comp)
				}
			}
		}
	}

	// Also get global completions
	globalComps, err := c.getGlobalCompletions(prefix, seen)
	if err == nil {
		completions = append(completions, globalComps...)
	}

	// Sort by name
	sort.Slice(completions, func(i, j int) bool {
		return completions[i].Text < completions[j].Text
	})

	// Limit results
	if len(completions) > 50 {
		completions = completions[:50]
	}

	return completions, nil
}

// getGlobalCompletions returns completions from globalThis
func (c *Completer) getGlobalCompletions(prefix string, seen map[string]bool) ([]Completion, error) {
	// Evaluate globalThis to get its properties
	result, err := c.evaluator.EvaluateGlobal("globalThis")
	if err != nil {
		return nil, err
	}

	if result.Result.ObjectID == "" {
		return nil, nil
	}

	props, err := c.evaluator.GetProperties(result.Result.ObjectID, true)
	if err != nil {
		return nil, err
	}

	var completions []Completion
	prefix = strings.ToLower(prefix)

	for _, prop := range props {
		lowerName := strings.ToLower(prop.Name)
		if !strings.HasPrefix(lowerName, prefix) {
			continue
		}
		if seen[prop.Name] {
			continue
		}
		seen[prop.Name] = true

		comp := Completion{
			Text:  prop.Name,
			Label: prop.Name,
		}

		switch prop.Value.Type {
		case "function":
			comp.Type = "function"
			comp.Label = prop.Name + "()"
			comp.Description = "function"
		default:
			comp.Type = "variable"
			comp.Description = formatTypeDescription(prop.Value)
		}

		completions = append(completions, comp)
	}

	return completions, nil
}

// GetObjectCompletions returns all properties of an object for bracket completion
func (c *Completer) GetObjectCompletions(objectExpr string) ([]Completion, error) {
	return c.getPropertyCompletions(objectExpr, "")
}

// formatTypeDescription creates a short description of a value
func formatTypeDescription(obj RemoteObject) string {
	switch obj.Type {
	case "undefined":
		return "undefined"
	case "null":
		return "null"
	case "boolean":
		if v, ok := obj.Value.(bool); ok {
			if v {
				return "true"
			}
			return "false"
		}
		return "boolean"
	case "number":
		if obj.Description != "" {
			return obj.Description
		}
		if v, ok := obj.Value.(float64); ok {
			return formatNumber(v)
		}
		return "number"
	case "string":
		if s, ok := obj.Value.(string); ok {
			if len(s) > 20 {
				return `"` + s[:17] + `..."`
			}
			return `"` + s + `"`
		}
		return "string"
	case "object":
		if obj.ClassName != "" {
			return obj.ClassName
		}
		if obj.Subtype != "" {
			return obj.Subtype
		}
		return "object"
	case "function":
		return "function"
	case "symbol":
		if obj.Description != "" {
			return obj.Description
		}
		return "symbol"
	case "bigint":
		if obj.Description != "" {
			return obj.Description
		}
		return "bigint"
	default:
		return obj.Type
	}
}

func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return json.Number(strings.TrimSuffix(strings.TrimSuffix(
			strings.TrimRight(strings.TrimRight(
				json.Number(strings.TrimSuffix(
					strings.TrimSuffix("%f", ".0"), "0")).String(),
				"0"), "."), "0"), ".")).String()
	}
	if v > 1e10 || v < -1e10 {
		return "number"
	}
	s := strings.TrimRight(strings.TrimRight(
		formatFloat(v), "0"), ".")
	return s
}

func formatFloat(v float64) string {
	// Simple formatting
	if v == float64(int64(v)) {
		return string(json.Number(string(rune(int64(v) + '0'))))
	}
	return "number"
}

// getLastIdentifier extracts the last identifier from text
// e.g., "foo + bar.ba" -> "bar.ba" if completing property, or "ba" for variable
func getLastIdentifier(text string) string {
	// Walk backwards to find start of identifier
	runes := []rune(text)
	end := len(runes)
	start := end

	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		if isIdentChar(r) || r == '.' || r == '_' || r == '$' {
			start = i
		} else {
			break
		}
	}

	if start >= end {
		return ""
	}

	result := string(runes[start:end])

	// For variable completion, extract just the last part after operators
	for _, sep := range []string{" ", "(", ",", "+", "-", "*", "/", "=", "!", "<", ">", "&", "|", "?"} {
		if idx := strings.LastIndex(result, sep); idx >= 0 {
			result = result[idx+1:]
		}
	}

	// If there's a dot, this should be handled by property completion
	if strings.Contains(result, ".") {
		// Return just the part after the last dot for prefix matching
		parts := strings.Split(result, ".")
		return parts[len(parts)-1]
	}

	return result
}

func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '$'
}
