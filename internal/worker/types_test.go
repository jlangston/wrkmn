package worker

import (
	"testing"

	"wrkmn/internal/config"
)

func TestEffectiveDevModeDefaultsToRemoteBindings(t *testing.T) {
	w := &Worker{}
	if got := w.EffectiveDevMode(); got != config.DevModeRemoteBindings {
		t.Fatalf("EffectiveDevMode() = %q, want %q", got, config.DevModeRemoteBindings)
	}
}

func TestSupportsDebugging(t *testing.T) {
	cases := []struct {
		mode config.DevMode
		want bool
	}{
		{"", true},
		{config.DevModeLocal, true},
		{config.DevModeRemoteBindings, true},
		{config.DevModeRemote, false},
	}
	for _, c := range cases {
		w := &Worker{DevMode: c.mode}
		if got := w.SupportsDebugging(); got != c.want {
			t.Fatalf("SupportsDebugging(%q) = %v, want %v", c.mode, got, c.want)
		}
	}
}
