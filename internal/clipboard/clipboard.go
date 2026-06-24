// Package clipboard copies text to the terminal's clipboard using OSC 52
// escape sequences.
//
// OSC 52 is interpreted by the terminal emulator itself, so it works over SSH
// and through tmux/screen — unlike helpers that shell out to pbcopy/xclip/
// wl-copy, which only reach the machine the process runs on. tmux forwards the
// sequence to the outer terminal when set-clipboard is "on" or "external"
// (the modern default); add `set -g set-clipboard on` to ~/.tmux.conf if
// copies don't reach your local clipboard.
package clipboard

import (
	"os"
	"strings"

	"github.com/aymanbagabas/go-osc52/v2"
)

// Copy writes text to the system clipboard via an OSC 52 escape sequence.
//
// The sequence is written to os.Stderr rather than os.Stdout: in a Bubble Tea
// program the renderer owns stdout for the alt-screen, so writing the escape
// to stderr (the same tty, a different fd) avoids interleaving with rendered
// frames. The sequence is invisible, so it does not disturb the display.
//
// OSC 52 is fire-and-forget — the terminal does not acknowledge it — so there
// is no error to return. An empty string is ignored to avoid clearing the
// clipboard.
func Copy(s string) {
	if s == "" {
		return
	}

	seq := osc52.New(s)
	// screen needs its own DCS wrapping; tmux captures plain OSC 52 directly.
	if strings.HasPrefix(os.Getenv("TERM"), "screen") {
		seq = seq.Screen()
	}
	_, _ = seq.WriteTo(os.Stderr)
}
