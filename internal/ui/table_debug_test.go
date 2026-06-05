package ui

import "testing"

func TestFormatConsoleEvent(t *testing.T) {
	tests := []struct {
		name      string
		data      any
		wantText  string
		wantError bool
	}{
		{
			name: "log with value args",
			data: map[string]any{
				"type": "log",
				"args": []any{
					map[string]any{"value": "hello"},
					map[string]any{"value": float64(42)},
				},
			},
			wantText:  "hello 42",
			wantError: false,
		},
		{
			name: "error type routes to error",
			data: map[string]any{
				"type": "error",
				"args": []any{map[string]any{"value": "boom"}},
			},
			wantText:  "boom",
			wantError: true,
		},
		{
			name: "object via description",
			data: map[string]any{
				"type": "log",
				"args": []any{map[string]any{"description": "[object Object]"}},
			},
			wantText:  "[object Object]",
			wantError: false,
		},
		{
			name:     "non-map data",
			data:     "nope",
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, isErr := formatConsoleEvent(tt.data)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if isErr != tt.wantError {
				t.Errorf("isError = %v, want %v", isErr, tt.wantError)
			}
		})
	}
}

func TestFormatExceptionEvent(t *testing.T) {
	data := map[string]any{
		"exceptionDetails": map[string]any{
			"text": "Uncaught",
			"exception": map[string]any{
				"description": "TypeError: x is not a function",
			},
		},
	}
	got := formatExceptionEvent(data)
	want := "Uncaught TypeError: x is not a function"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if formatExceptionEvent("not a map") != "" {
		t.Fatalf("expected empty string for non-map input")
	}
}

func TestListenDebugEventsNilSession(t *testing.T) {
	if cmd := listenDebugEvents(nil); cmd != nil {
		t.Fatalf("expected nil cmd for nil session")
	}
}
