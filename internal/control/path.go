// Package control is the remote-control surface for a running wrkmn: a
// Unix-domain-socket server inside the TUI, a tiny JSON protocol, and the
// client `wrkmn ctl` uses. Requests are turned into messages for the Bubble
// Tea model so they travel the same handlers as the keybindings.
package control

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// EnvSocketOverride names the environment variable that pins the socket
// path on both the TUI and the ctl client.
const EnvSocketOverride = "WRKMN_CONTROL_SOCKET"

// InstanceID is the per-project identifier: the first 12 hex characters of
// sha256 over the normalized absolute base directory.
func InstanceID(absBaseDir string) string {
	sum := sha256.Sum256([]byte(absBaseDir))
	return hex.EncodeToString(sum[:])[:12]
}

// NormalizeBaseDir turns any spelling of the project directory (relative,
// trailing slash, through a symlink) into the one canonical absolute path
// both ends hash. Symlinks are resolved when the directory exists.
func NormalizeBaseDir(baseDir string) (string, error) {
	if baseDir == "" {
		baseDir = "."
	}
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return abs, nil
}

// SocketPath derives the control socket path for a project directory.
//
//	override != ""      -> override (WRKMN_CONTROL_SOCKET)
//	xdgRuntimeDir != "" -> $XDG_RUNTIME_DIR/wrkmn/<id>.sock
//	otherwise           -> <os.TempDir()>/wrkmn-<uid>/<id>.sock
func SocketPath(baseDir, override, xdgRuntimeDir string, uid int) (string, error) {
	if override != "" {
		return override, nil
	}
	abs, err := NormalizeBaseDir(baseDir)
	if err != nil {
		return "", err
	}
	id := InstanceID(abs)
	if xdgRuntimeDir != "" {
		return filepath.Join(xdgRuntimeDir, "wrkmn", id+".sock"), nil
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("wrkmn-%d", uid), id+".sock"), nil
}

// DefaultSocketPath is SocketPath fed from the process environment.
func DefaultSocketPath(baseDir string) (string, error) {
	return SocketPath(baseDir, os.Getenv(EnvSocketOverride), os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
}
