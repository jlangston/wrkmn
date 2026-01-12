package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"wrkmn/internal/worker"
)

func TestNewScriptPopup(t *testing.T) {
	popup := NewScriptPopup()
	assert.NotNil(t, popup)
	assert.False(t, popup.Visible())
	assert.False(t, popup.running)
}

func TestScriptPopup_Show(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "test-worker",
		Path: "/tmp/test",
		Scripts: map[string]string{
			"build":   "tsc",
			"dev":     "wrangler dev",
			"test":    "vitest",
			"_config": "internal", // Should be filtered out
		},
	}

	popup.Show(w)

	assert.True(t, popup.Visible())
	assert.Equal(t, w, popup.worker)
	assert.Equal(t, 0, popup.cursor)
	assert.Equal(t, 0, popup.scroll)
	assert.False(t, popup.running)

	// Should have 3 scripts (excluding _config)
	assert.Equal(t, 3, len(popup.scripts))

	// Scripts should be sorted alphabetically
	assert.Equal(t, "build", popup.scripts[0])
	assert.Equal(t, "dev", popup.scripts[1])
	assert.Equal(t, "test", popup.scripts[2])
}

func TestScriptPopup_ShowNoScripts(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name:    "test-worker",
		Path:    "/tmp/test",
		Scripts: map[string]string{},
	}

	popup.Show(w)

	assert.True(t, popup.Visible())
	assert.Equal(t, 0, len(popup.scripts))
}

func TestScriptPopup_Hide(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name:    "test-worker",
		Scripts: map[string]string{"build": "tsc"},
	}

	popup.Show(w)
	assert.True(t, popup.Visible())

	popup.Hide()
	assert.False(t, popup.Visible())
}

func TestScriptPopup_Visible(t *testing.T) {
	popup := NewScriptPopup()
	assert.False(t, popup.Visible())

	w := &worker.Worker{
		Name:    "test",
		Scripts: map[string]string{"build": "tsc"},
	}

	popup.Show(w)
	assert.True(t, popup.Visible())

	popup.Hide()
	assert.False(t, popup.Visible())
}

func TestScriptPopup_SetSize(t *testing.T) {
	popup := NewScriptPopup()

	popup.SetSize(100, 50)

	assert.Equal(t, 100, popup.width)
	assert.Equal(t, 50, popup.height)
}

func TestScriptPopup_Update_Navigation(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "test",
		Scripts: map[string]string{
			"a": "cmd",
			"b": "cmd",
			"c": "cmd",
		},
	}

	popup.Show(w)
	popup.SetSize(80, 30)

	assert.Equal(t, 0, popup.cursor)

	// Navigate down with j
	popup.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, popup.cursor)

	// Navigate down with arrow
	popup.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, popup.cursor)

	// Should not go past end
	popup.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, popup.cursor)

	// Navigate up with k
	popup.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 1, popup.cursor)

	// Navigate up with arrow
	popup.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, popup.cursor)

	// Should not go below 0
	popup.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, popup.cursor)
}

func TestScriptPopup_Update_GoToTop(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "test",
		Scripts: map[string]string{
			"a": "cmd",
			"b": "cmd",
			"c": "cmd",
		},
	}

	popup.Show(w)
	popup.SetSize(80, 30)
	popup.cursor = 2

	// Press 'g' to go to top
	popup.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	assert.Equal(t, 0, popup.cursor)
}

func TestScriptPopup_Update_GoToBottom(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "test",
		Scripts: map[string]string{
			"a": "cmd",
			"b": "cmd",
			"c": "cmd",
		},
	}

	popup.Show(w)
	popup.SetSize(80, 30)

	// Press 'G' to go to bottom
	popup.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	assert.Equal(t, 2, popup.cursor)
}

func TestScriptPopup_Update_Close_Q(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name:    "test",
		Scripts: map[string]string{"build": "tsc"},
	}

	popup.Show(w)
	assert.True(t, popup.Visible())

	popup.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.False(t, popup.Visible())
}

func TestScriptPopup_Update_Close_Escape(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name:    "test",
		Scripts: map[string]string{"build": "tsc"},
	}

	popup.Show(w)
	assert.True(t, popup.Visible())

	popup.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.False(t, popup.Visible())
}

func TestScriptPopup_Update_NotVisible(t *testing.T) {
	popup := NewScriptPopup()

	// When not visible, update should return nil
	cmd := popup.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Nil(t, cmd)
}

func TestScriptPopup_View_Empty(t *testing.T) {
	popup := NewScriptPopup()

	// Not visible - should return empty
	view := popup.View()
	assert.Equal(t, "", view)
}

func TestScriptPopup_View_WithScripts(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "my-worker",
		Scripts: map[string]string{
			"build": "tsc",
			"dev":   "wrangler dev",
		},
	}

	popup.Show(w)
	popup.SetSize(80, 30)

	view := popup.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "my-worker")
	assert.Contains(t, view, "build")
	assert.Contains(t, view, "dev")
}

func TestScriptPopup_View_NoScripts(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name:    "empty-worker",
		Scripts: map[string]string{},
	}

	popup.Show(w)
	popup.SetSize(80, 30)

	view := popup.View()
	assert.Contains(t, view, "No scripts found")
}

func TestScriptPopup_FilterInternalScripts(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "test",
		Scripts: map[string]string{
			"build":   "tsc",
			"_config": "internal",
		},
	}

	popup.Show(w)

	// Should only have "build", not "_config"
	assert.Equal(t, 1, len(popup.scripts))
	assert.Equal(t, "build", popup.scripts[0])
}

func TestScriptPopup_SortsScripts(t *testing.T) {
	popup := NewScriptPopup()

	w := &worker.Worker{
		Name: "test",
		Scripts: map[string]string{
			"zebra": "cmd",
			"alpha": "cmd",
			"mango": "cmd",
		},
	}

	popup.Show(w)

	assert.Equal(t, []string{"alpha", "mango", "zebra"}, popup.scripts)
}

func TestScriptPopup_GoToBottom_EmptyScripts(t *testing.T) {
	popup := NewScriptPopup()
	w := &worker.Worker{
		Name:    "test-worker",
		Scripts: map[string]string{},
	}
	popup.Show(w)
	popup.SetSize(80, 30)

	// Verify scripts is empty
	if len(popup.scripts) != 0 {
		t.Errorf("Expected 0 scripts, got %d", len(popup.scripts))
	}

	// Press G to go to bottom - should not cause cursor to become -1
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	popup.Update(msg)

	// Cursor should stay at 0, not become -1
	if popup.cursor != 0 {
		t.Errorf("Expected cursor 0, got %d", popup.cursor)
	}
}
