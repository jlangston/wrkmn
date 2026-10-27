package control

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInstanceIDIsFirst12HexOfSHA256(t *testing.T) {
	// printf '/srv/app' | sha256sum | cut -c1-12
	if got := InstanceID("/srv/app"); got != "dae668e4084f" {
		t.Fatalf("InstanceID(/srv/app) = %q, want dae668e4084f", got)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(InstanceID("/anything")) {
		t.Fatalf("InstanceID must be 12 lowercase hex chars")
	}
	if InstanceID("/a") == InstanceID("/b") {
		t.Fatal("different dirs must yield different ids")
	}
}

func TestSocketPathUsesXDGRuntimeDirWhenSet(t *testing.T) {
	dir := t.TempDir()
	got, err := SocketPath(dir, "", "/run/user/1000", 1000)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.EvalSymlinks(dir)
	want := filepath.Join("/run/user/1000", "wrkmn", InstanceID(abs)+".sock")
	if got != want {
		t.Fatalf("SocketPath = %q, want %q", got, want)
	}
}

func TestSocketPathFallsBackToTmpUIDDir(t *testing.T) {
	dir := t.TempDir()
	got, err := SocketPath(dir, "", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.EvalSymlinks(dir)
	want := filepath.Join(os.TempDir(), "wrkmn-1000", InstanceID(abs)+".sock")
	if got != want {
		t.Fatalf("SocketPath = %q, want %q", got, want)
	}
}

func TestSocketPathEnvOverrideWins(t *testing.T) {
	got, err := SocketPath(t.TempDir(), "/custom/ctl.sock", "/run/user/1000", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/custom/ctl.sock" {
		t.Fatalf("SocketPath = %q, want override", got)
	}
}

// Both sides of the socket derive the path independently (the TUI from
// cfg.BaseDir, the ctl client from --config/cwd), so every spelling of the
// same directory must land on the same socket.
func TestSocketPathIsStableAcrossDirSpellings(t *testing.T) {
	dir := t.TempDir()
	rel, err := filepath.Rel(mustGetwd(t), dir)
	if err != nil {
		t.Skip("temp dir not relativizable from cwd")
	}
	spellings := []string{dir, dir + "/", filepath.Join(dir, ".", "sub", ".."), rel}
	first, err := SocketPath(spellings[0], "", "/run/user/1000", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range spellings[1:] {
		got, err := SocketPath(s, "", "/run/user/1000", 1000)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if got != first {
			t.Fatalf("SocketPath(%q) = %q, want %q", s, got, first)
		}
	}
	if strings.Contains(first, "..") {
		t.Fatalf("path not cleaned: %q", first)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
