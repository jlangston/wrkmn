package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCompletionPrefix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		cursorPos int
		expected  string
	}{
		{"simple identifier", "foo", 3, "foo"},
		{"partial identifier", "res", 3, "res"},
		{"after dot", "response.bo", 11, "bo"},
		{"at dot", "response.", 9, ""},
		{"middle of identifier", "foobar", 3, "foo"},
		{"after space", "x + bar", 7, "bar"},
		{"after equals", "x = val", 7, "val"},
		{"empty input", "", 0, ""},
		{"cursor at start", "foo", 0, ""},
		{"underscore prefix", "_priv", 5, "_priv"},
		{"dollar prefix", "$el", 3, "$el"},
		{"after open paren", "log(msg", 7, "msg"},
		{"with numbers", "var1", 4, "var1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCompletionPrefix(tt.input, tt.cursorPos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCompletionChar(t *testing.T) {
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
		{'[', false},
		{']', false},
		{',', false},
		{':', false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isCompletionChar(tt.char), "isCompletionChar(%q)", tt.char)
	}
}

func TestGetCompletionPrefix_EdgeCases(t *testing.T) {
	// Cursor beyond input length
	result := getCompletionPrefix("foo", 10)
	assert.Equal(t, "foo", result)

	// Unicode characters
	result = getCompletionPrefix("日本語", 3)
	assert.Equal(t, "", result) // Non-ASCII not valid identifiers

	// Mixed valid/invalid
	result = getCompletionPrefix("foo.bar.baz", 11)
	assert.Equal(t, "baz", result)

	// Operators
	result = getCompletionPrefix("a + b * c", 9)
	assert.Equal(t, "c", result)
}
