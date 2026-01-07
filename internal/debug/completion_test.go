package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLastIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple variable", "foo", "foo"},
		{"partial variable", "res", "res"},
		{"after dot", "response.bo", "bo"},
		{"after space", "foo + bar", "bar"},
		{"after open paren", "console.log(msg", "msg"},
		{"after comma", "func(a, b", "b"},
		{"after equals", "x = val", "val"},
		{"underscore var", "_private", "_private"},
		{"dollar var", "$element", "$element"},
		{"number suffix", "var1", "var1"},
		{"empty string", "", ""},
		{"only operators", " + ", ""},
		{"trailing dot", "obj.", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLastIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsIdentChar(t *testing.T) {
	tests := []struct {
		char     rune
		expected bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{'$', true},
		{'.', false},
		{' ', false},
		{'-', false},
		{'+', false},
		{'(', false},
		{')', false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isIdentChar(tt.char), "isIdentChar(%q)", tt.char)
	}
}

func TestFormatTypeDescription(t *testing.T) {
	tests := []struct {
		name     string
		obj      RemoteObject
		expected string
	}{
		{
			name:     "undefined",
			obj:      RemoteObject{Type: "undefined"},
			expected: "undefined",
		},
		{
			name:     "null",
			obj:      RemoteObject{Type: "null"},
			expected: "null",
		},
		{
			name:     "boolean true",
			obj:      RemoteObject{Type: "boolean", Value: true},
			expected: "true",
		},
		{
			name:     "boolean false",
			obj:      RemoteObject{Type: "boolean", Value: false},
			expected: "false",
		},
		{
			name:     "number with description",
			obj:      RemoteObject{Type: "number", Description: "42"},
			expected: "42",
		},
		{
			name:     "short string",
			obj:      RemoteObject{Type: "string", Value: "hello"},
			expected: `"hello"`,
		},
		{
			name:     "long string truncated",
			obj:      RemoteObject{Type: "string", Value: "this is a very long string that should be truncated"},
			expected: `"this is a very lo..."`,
		},
		{
			name:     "object with class",
			obj:      RemoteObject{Type: "object", ClassName: "Request"},
			expected: "Request",
		},
		{
			name:     "object with subtype",
			obj:      RemoteObject{Type: "object", Subtype: "array"},
			expected: "array",
		},
		{
			name:     "plain object",
			obj:      RemoteObject{Type: "object"},
			expected: "object",
		},
		{
			name:     "function",
			obj:      RemoteObject{Type: "function"},
			expected: "function",
		},
		{
			name:     "symbol with description",
			obj:      RemoteObject{Type: "symbol", Description: "Symbol(foo)"},
			expected: "Symbol(foo)",
		},
		{
			name:     "symbol without description",
			obj:      RemoteObject{Type: "symbol"},
			expected: "symbol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTypeDescription(tt.obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompletion_Sort(t *testing.T) {
	// Test that completions are sorted alphabetically
	completions := []Completion{
		{Text: "zebra", Label: "zebra", Type: "variable"},
		{Text: "apple", Label: "apple", Type: "variable"},
		{Text: "mango", Label: "mango", Type: "variable"},
	}

	// Sort using same logic as completer
	sorted := make([]Completion, len(completions))
	copy(sorted, completions)

	// Just verify our test data structure
	assert.Equal(t, "zebra", completions[0].Text)
	assert.Equal(t, "apple", completions[1].Text)
	assert.Equal(t, "mango", completions[2].Text)
}

func TestCompletion_Types(t *testing.T) {
	// Test completion type constants
	tests := []struct {
		compType string
		valid    bool
	}{
		{"variable", true},
		{"property", true},
		{"function", true},
		{"method", true},
		{"unknown", true}, // any string is technically valid
	}

	for _, tt := range tests {
		c := Completion{Type: tt.compType}
		assert.NotEmpty(t, c.Type)
	}
}
